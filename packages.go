package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/minio/minio-go"
	"github.com/minio/minio/pkg/console"
)

var (
	normaliseRe        = regexp.MustCompile(`[-_.]+`)
	pythonVersion      = regexp.MustCompile(`-py(\d\.?\d?)`)
	pkgNameVersion     = regexp.MustCompile(`([a-z0-9_]+([.-][a-z_][a-z0-9_]*)*)-([a-z0-9_.+-]+)`)
	pathSeparator      = "/"
	packageListFile    = "packages.json"
	sourceExtensions   = []string{".tar.gz", ".tar.bz2", ".tar", ".zip", ".tgz", ".tbz"}
	binaryExtensions   = []string{".egg", ".exe", ".whl"}
	excludedExtensions = ".pdf"
)

type pkg struct {
	Name     string `json:"name"`
	FileName string `json:"filename"`
	Version  string `json:"version"`
	PyVer    string `json:"pyver"`
	URL      string `json:"url"`
	MD5      string `json:"md5_digest"`
	Summary  string `json:"summary"`
}

// PkgError represents a package specific error when uploading, downloading up updating a package
type PkgError uint

const (
	// PkgUnknown is the default/unknown error state for pkg
	PkgUnknown = PkgError(iota)

	// AlreadyExists means that the package name and version combination already exists on the server
	AlreadyExists

	// InvalidFormat means that the package the user is attempting to upload is incorrectly formatted
	InvalidFormat
)

func (e PkgError) Error() string {
	switch e {
	case 1:
		return "AlreadyExists"
	case 2:
		return "InvalidFormat"
	default:
		return "UnknownError"
	}
}

type pkgs map[string][]pkg

func (ps pkgs) GetLatestVersion(pkgName string) string {
	var rawVersions []string
	for _, v := range ps[pkgName] {
		rawVersions = append(rawVersions, v.Version)
	}
	if len(rawVersions) == 0 {
		return ""
	}
	if len(rawVersions) == 1 {
		return rawVersions[0]
	}

	vs := make([]*semver.Version, len(rawVersions))
	for i, r := range rawVersions {
		v, err := semver.NewVersion(r)
		if err != nil {
			console.Errorf("Error parsing version: %s", err)
			continue
		}
		vs[i] = v
	}
	sort.Sort(sort.Reverse(semver.Collection(vs)))
	if vs[0] != nil {
		return vs[0].Original()
	}
	return ""
}

func (s *server) removePackage(name, version string) error {
	for i, pkg := range s.packages[name] {
		if pkg.Name == name && pkg.Version == version {
			s.packages[name] = append(s.packages[name][0:i], s.packages[name][i+1:]...)
		}
	}
	return s.writePackagesJSON()
}

func (s *server) addPackage(p pkg) error {
	// Let's make sure we refresh our in-memory package list first
	// so we don't upload a package twice in-case the package.json
	// has been manually edited

	err := s.readPackagesJSON()
	if err != nil {
		return err
	}
	if _, exists := s.packages[p.Name]; !exists {
		s.packages[p.Name] = []pkg{}
	}
	// Check if the version we're adding already exists
	for _, pkg := range s.packages[p.Name] {
		if pkg.Version == p.Version {
			return AlreadyExists
		}
	}
	s.packages[p.Name] = append(s.packages[p.Name], p)
	// Todo(LeoS): Make sure we don't add the package if the writePackageJSON function doesn't succeed.
	return s.writePackagesJSON()
}

func (s *server) writePackagesJSON() error {
	pkgsJSON, err := json.Marshal(s.packages)
	if err != nil {
		return err
	}
	r := bytes.NewReader(pkgsJSON)
	_, err = s.s3.PutObject(s.s3cfg.bucket, packageListFile, r, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return err
	}
	return nil
}

func (s *server) readPackagesJSON() error {
	_, err := s.s3.StatObject(s.s3cfg.bucket, packageListFile, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "AccessDenied" {
			return AccessDenied
		}
		if errResponse.Code == "NoSuchBucket" {
			return NoSuchBucket
		}
		if errResponse.Code == "InvalidBucketName" {
			return InvalidBucketName
		}
		if errResponse.Code == "NoSuchKey" {
			return NoSuchKey
		}
		return err
	}
	o, err := s.s3.GetObject(s.s3cfg.bucket, packageListFile, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	pkgs := &pkgs{}
	buf := new(bytes.Buffer)
	buf.ReadFrom(o)
	err = json.Unmarshal(buf.Bytes(), pkgs)
	if err != nil {
		return err
	}
	s.packages = *pkgs
	return nil
}

// newPkg takes Filename, version from POST form and S3 location and returns a "pkg" struct
func newPkg(fileName, version, summary, md5, location string) pkg {
	pkg := parseFilename(fileName)
	pkg.FileName = fileName
	pkg.URL = fmt.Sprintf("/%s", location)
	pkg.Summary = summary
	pkg.MD5 = md5
	if pkg.Version != version {
		console.Infoln("Uploaded package filename and POST form have different versions. Using form value")
		console.Debugf("Form Version: %s\tFile Version: %s\n", version, pkg.Version)
		pkg.Version = version
	}
	return pkg
}

// Almost https://github.com/vsajip/distlib/blob/master/distlib/util.py#L839
// but doesn't support passing in a pkgName as a second arg to help parsing
// Also doesn't properly handle Wheels as far I know, but need more testing
func parseFilename(fileName string) pkg {
	p := pkg{}
	for _, ext := range append(binaryExtensions, sourceExtensions...) {
		if strings.HasSuffix(fileName, ext) {

			// Replace spaces with dashes and trim the extension
			trimmed := strings.TrimSuffix(strings.ReplaceAll(fileName, " ", "-"), ext)

			// Check for a specified python version ("-py2.7" for example).
			pyver := pythonVersion.FindStringSubmatch(trimmed)
			if len(pyver) != 0 {

				p.PyVer = pyver[1]
				// Get the start of the match ([0] is start of match, [1] is end of match)
				pyVerStart := pythonVersion.FindStringIndex(trimmed)[0]
				// Trim "-py..."
				trimmed = trimmed[:pyVerStart]
			}
			// Grab the rest of the info by looking for the package version and assuming the rest is the name
			pkgVer := pkgNameVersion.FindStringSubmatch(trimmed)

			p.Name = normalisePackageName(pkgVer[1])
			p.Version = pkgVer[3]

		}
	}
	return p
}

func normalisePackageName(name string) string {
	return strings.ToLower(normaliseRe.ReplaceAllString(name, "-"))
}
