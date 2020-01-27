package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/minio/minio/pkg/console"
)

type s3Config struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
}

var (
	endpoint  string
	accessKey string
	secretKey string
	port      string
	bucket    string
	debug     bool
)

func main() {
	if err := run(); err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}

func run() error {
	flag.StringVar(&endpoint, "endpoint", "http://localhost:9000", "S3 server endpoint")
	flag.StringVar(&accessKey, "accessKey", "", "Access key of S3 storage")
	flag.StringVar(&secretKey, "secretKey", "", "Secret key of S3 storage")
	flag.StringVar(&bucket, "bucket", "", "Bucket name which hosts static files")
	flag.StringVar(&port, "port", "8080", "Bind to a specific port")
	flag.BoolVar(&debug, "debug", false, "Enable debug logs")
	flag.Parse()

	if strings.TrimSpace(bucket) == "" {
		console.Fatalln(`Bucket name cannot be empty, please provide 'gopi -bucket "mybucket"'`)
	}
	if strings.TrimSpace(endpoint) == "" {
		console.Fatalln(`Endpoint cannot be empty, please provide 'gopi -endpoint "http://localhost:9000/"'`)
	}

	console.DebugPrint = debug

	cfg := s3Config{
		endpoint:  endpoint,
		bucket:    bucket,
		accessKey: accessKey,
		secretKey: secretKey,
	}
	s := newServer(cfg)
	console.Infof("Serving on port %s\n", port)
	return http.ListenAndServe("0.0.0.0:"+port, s)
}
