package main

func (s *server) routes() {
	// 	s.router.HandleFunc("/", s.HomeHandler())
	s.router.HandleFunc("/simple/", s.SimpleHandler()).Methods("GET")
	s.router.HandleFunc("/simple/{package}", s.SimpleHandler()).Methods("GET")

	// There's probably a nicer way of handling both of these endpoints without redirecting
	s.router.HandleFunc("/simple", s.UploadHandler()).Methods("POST")
	s.router.HandleFunc("/simple/", s.UploadHandler()).Methods("POST")

	return
}
