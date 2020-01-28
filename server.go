package main

import (
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/credentials"
	"github.com/minio/minio-go/pkg/s3utils"
)

type server struct {
	router    *mux.Router
	s3cfg     s3Config
	packages  pkgs
	s3        *minio.Client
	templates *template.Template
}

func newServer(s3cfg s3Config) (*server, error) {
	s := &server{}
	var err error
	// Make sure we connect to S3 before we start router as it depends on S3 connections
	s.s3cfg = s3cfg
	err = s.S3Connect()
	if err != nil {
		return s, err
	}
	s.packages = make(map[string][]pkg)

	r := mux.NewRouter()
	s.router = r
	s.routes()

	err = s.parseTemplates()
	if err != nil {
		return s, err
	}
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) S3Connect() error {

	if strings.TrimSpace(s.s3cfg.bucket) == "" {
		return errors.New(`Bucket name cannot be empty, please provide 'gopi -bucket "mybucket"'`)
	}

	u, err := url.Parse(s.s3cfg.endpoint)
	if err != nil {
		return err
	}

	// Chains all credential types, in the following order:
	//  - AWS env vars (i.e. AWS_ACCESS_KEY_ID)
	//  - AWS creds file (i.e. AWS_SHARED_CREDENTIALS_FILE or ~/.aws/credentials)
	//  - IAM profile based credentials. (performs an HTTP
	//    call to a pre-defined endpoint, only valid inside
	//    configured ec2 instances)
	var defaultAWSCredProviders = []credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.FileAWSCredentials{},
		&credentials.IAM{
			Client: &http.Client{
				Transport: NewCustomHTTPTransport(),
			},
		},
		&credentials.EnvMinio{},
	}
	if accessKey != "" && secretKey != "" {
		defaultAWSCredProviders = []credentials.Provider{
			&credentials.Static{
				Value: credentials.Value{
					AccessKeyID:     s.s3cfg.accessKey,
					SecretAccessKey: s.s3cfg.secretKey,
				},
			},
		}
	}

	// If we see an Amazon S3 endpoint, then we use more ways to fetch backend credentials.
	// Specifically IAM style rotating credentials are only supported with AWS S3 endpoint.
	creds := credentials.NewChainCredentials(defaultAWSCredProviders)

	client, err := minio.NewWithOptions(u.Host, &minio.Options{
		Creds:        creds,
		Secure:       u.Scheme == "https",
		Region:       s3utils.GetRegionFromURL(*u),
		BucketLookup: minio.BucketLookupAuto,
	})
	if err != nil {
		return err
	}
	s.s3 = client
	return nil
}

// NewCustomHTTPTransport returns a new http configuration
// used while communicating with the cloud backends.
// This sets the value for MaxIdleConnsPerHost from 2 (go default)
// to 100.
func NewCustomHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          1024,
		MaxIdleConnsPerHost:   1024,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}
}

func (s *server) parseTemplates() error {
	templates, err := template.ParseGlob("templates/*.tpl.html")
	if err != nil {
		return fmt.Errorf("Failed to parse templates, %s", err.Error())
	}
	s.templates = templates
	return nil
}
