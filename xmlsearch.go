package main

import (
	"net/http"
	"strings"

	"github.com/minio/minio/pkg/console"
)

type PackageSearchArgs struct {
	Query struct {
		Name    []string
		Summary []string
	}
	Operator string
}

type PackageSearchReply struct {
	Packages []PackageVersion
}

type PackageVersion struct {
	Name     string `xml:"name"`
	Summary  string `xml:"summary"`
	Version  string `xml:"version"`
	Ordering bool   `xml:"_pypi_ordering"`
}

type XMLSearch struct {
	server *server
}

func newXMLSearch(server *server) *XMLSearch {
	return &XMLSearch{
		server: server,
	}
}

func (h *XMLSearch) Search(r *http.Request, args *PackageSearchArgs, reply *PackageSearchReply) error {
	console.Debugf("Query is: %+v\n", args)
	nameQuery := args.Query.Name
	summaryQuery := args.Query.Summary
	ps := h.server.packages

	matched := make(map[string]pkg, 0)

	// First look for summaries and add all versions of the packages who's
	// summary matches
	matchingVersions := pkgs{}
	for _, searchSummary := range summaryQuery {
		for _, packageVersions := range ps {
			for _, p := range packageVersions {
				if strings.Contains(strings.ToLower(p.Summary), strings.ToLower(searchSummary)) {
					matchingVersions = append(matchingVersions, p)
				}
			}
		}
	}
	// From the list of matched package versions, get the latest matching version
	latestMatchedVersions := matchingVersions.GetLatestVersionPackage()
	matched[latestMatchedVersions.Name] = latestMatchedVersions

	// Now search for a name match and overwrite the previous query if
	// we find a name match instead.
	for _, searchName := range nameQuery {
		for packageName, packageVersions := range ps {
			if strings.Contains(strings.ToLower(packageName), strings.ToLower(searchName)) {
				matched[packageName] = packageVersions.GetLatestVersionPackage()
			}
		}
	}
	if len(matched) == 0 {
		return nil
	}
	replyPkgList := []PackageVersion{}
	for _, p := range matched {
		pv := PackageVersion{
			Name:     p.Name,
			Summary:  p.Summary,
			Version:  p.Version,
			Ordering: false,
		}
		replyPkgList = append(replyPkgList, pv)
	}
	reply.Packages = replyPkgList
	console.Debugf("Reply is: %+v\n", reply)
	return nil
}
