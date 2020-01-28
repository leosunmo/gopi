package main

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go"
	"github.com/minio/minio/pkg/console"
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
			// Save to package list in server
			err = s.addPackage(newPkg(header.Filename, version, summary, md5, s3Location))
			if err != nil {
				if errors.Is(err, AlreadyExists) {
					console.Errorf("Package %s, version %s already exists\n", header.Filename, version)
					http.Error(w, fmt.Sprintf("Package already exists"), http.StatusConflict)
					return
				}
				console.Errorf("Failed to write package list, err: %s\n", header.Filename, err.Error())
				http.Error(w, fmt.Sprintf("Failed to upload file %s", header.Filename), http.StatusInternalServerError)
				return
			}
			uploadedSize, err := s.s3.PutObject(s.s3cfg.bucket, s3Location, file, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
			if err != nil {
				console.Errorf("Failed to upload file %s, err %s\n", header.Filename, err.Error())
				http.Error(w, fmt.Sprintf("Failed to upload file %s", header.Filename), http.StatusInternalServerError)
				return
			}
			console.Infof("Put object %s, size %d\n", header.Filename, uploadedSize)
		}
	}
}
