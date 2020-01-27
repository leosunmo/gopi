package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go"
	"github.com/minio/minio/pkg/console"
)

const (
	NoType = S3Error(iota)
	AccessDenied
	NoSuchBucket
	InvalidBucketName
	NoSuchKey
)

// S3Error describes a bucket, object or network error when connecting to S3
type S3Error uint

// Error returns the mssage of a customError
func (e S3Error) Error() string {
	switch e {
	case 1:
		return "AccessDenied"
	case 2:
		return "NoSuchBucket"
	case 3:
		return "InvalidBucketName"
	case 4:
		return "NoSuchKey"
	default:
		return "UnknownError"
	}
}

type pkg struct {
	Name     string `json:"name"`
	FileName string `json:"filename"`
	Version  string `json:"version"`
	PyVer    string `json:"pyver"`
	URL      string `json:"url"`
	MD5      string `json:"md5_digest"`
	Summary  string `json:"summary"`
}

type pkgs map[string][]pkg

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

func (s *server) SimpleHandler() http.HandlerFunc {
	err := s.readPackagesJSON()
	if err != nil {
		if !errors.Is(err, NoSuchKey) {
			console.Fatalf("Failed to read package JSON from bucket: %s\n", err.Error())
		}
	}
	list, err := template.ParseFiles("templates/packages.tpl.html")
	if err != nil {
		console.Fatalf("Failed to parse package list HTML template: %s\n", err.Error())
	}
	singlePackage, err := template.ParseFiles("templates/package.tpl.html")
	if err != nil {
		console.Fatalf("Failed to parse package HTML template: %s\n", err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if vars["package"] == "" {
			list.Execute(w, s.packages)
		} else {
			singlePackage.Execute(w, s.packages[vars["package"]])
		}
		return
	}
}

// Path is "/simple(/)?" POSTs only
func (s *server) UploadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		console.Debugf("Path: %s\n", r.URL.Path)
		vars := mux.Vars(r)
		action := r.FormValue(":action")
		switch action {
		case "file_upload":
			console.Debugf("Action is file_upload")
			console.Debugf("Path: %s\tPackage: %s\tVersion: %s\n", vars["pkg"], r.FormValue("name"), r.FormValue("version"))
			err := r.ParseForm()
			if err != nil {
				console.Errorf("Failed to parse form, err: %s\n", err.Error())
				http.Error(w, fmt.Sprintf("Failed to parse form"), http.StatusInternalServerError)
				return
			}
			console.Debugf("FormValues:\n%+v\n\n", r.PostForm)
			packageName := normalisePackageName(r.FormValue("name"))
			console.Debugf("packageName %s\n", packageName)
			r.ParseMultipartForm(32 << 20) // limit your max input length!

			version := r.FormValue("version")
			summary := r.FormValue("summary")
			md5 := r.FormValue("md5_digest")

			file, header, err := r.FormFile("content")

			if packageName == "" {
				console.Infoln("No package name detected in form, using filename as package name")
				name := strings.Split(header.Filename, ".")
				packageName = normalisePackageName(name[0])
			}
			s3Location := fmt.Sprintf("%s%s%s", packageName, pathSeparator, header.Filename)

			uploadedSize, err := s.s3.PutObject(s.s3cfg.bucket, s3Location, file, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
			if err != nil {
				console.Errorf("Failed to upload file %s, err %s\n", header.Filename, err.Error())
				http.Error(w, fmt.Sprintf("Failed to upload file %s", header.Filename), http.StatusInternalServerError)
				return
			}
			console.Infof("Put object %s, size %d\n", header.Filename, uploadedSize)
			// Save to package list in server
			err = s.addPackage(getPkg(header.Filename, version, summary, md5, s3Location))
			if err != nil {
				console.Errorf("Failed to write package list, err: %s\n", header.Filename, err.Error())
				http.Error(w, fmt.Sprintf("Failed to upload file %s", header.Filename), http.StatusInternalServerError)
				return
			}
		}
	}
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
	if _, exists := s.packages[p.Name]; !exists {
		s.packages[p.Name] = []pkg{}
	}
	s.packages[p.Name] = append(s.packages[p.Name], p)
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
	console.Debugf("%+v\n", *o)
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

// getPkg takes Filename, version from POST form and S3 location and returns a "pkg" struct
func getPkg(fileName, version, summary, md5, location string) pkg {
	pkg := parseFilename(fileName)
	pkg.FileName = fileName
	pkg.URL = fmt.Sprintf("/api/%s", location)
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
