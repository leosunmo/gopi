package main

import "net/http"

func (s *server) routes() {
	s.router.PathPrefix("/assets").Handler(http.FileServer(http.Dir("./assets/")))

	s.router.HandleFunc("/", s.HomeHandler())
	s.router.HandleFunc("/package/{package}", s.DetailsHandler())

	s.router.HandleFunc("/simple/", s.SimpleHandler()).Methods("GET")
	s.router.HandleFunc("/simple/{package}/", s.SimpleHandler()).Methods("GET")

	// There's probably a nicer way of handling both of these endpoints without redirecting
	s.router.HandleFunc("/simple", s.UploadHandler()).Methods("POST")
	s.router.HandleFunc("/simple/", s.UploadHandler()).Methods("POST")

	s.router.HandleFunc("/api/{package}/{file}", s.DownloadHander())
	return
}
