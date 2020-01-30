package main

type s3Config struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
}

// S3Error describes a bucket, object or network error when connecting to S3
type S3Error uint

const (
	// S3Unknown is the default/unknown error state for S3Error
	S3Unknown = S3Error(iota)

	// AccessDenied means the server does not have access to the object/bucket
	AccessDenied

	// NoSuchBucket means that the specified bucket doesn't exist
	NoSuchBucket

	// InvalidBucketName means the provided bucket name is not the correct format
	InvalidBucketName

	// NoSuchKey means the object does not exist in the bucket, essentially "file not found"
	NoSuchKey
)

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
