package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3GetObjectAPI defines the interface for the GetObject function.
// We use this interface to test the function using a mocked service.
type S3GetObjectAPI interface {
	GetObject(ctx context.Context,
		params *s3.GetObjectInput,
		optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// GetObject retrieves objects from Amazon S3
func GetObject(c context.Context, api S3GetObjectAPI, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	return api.GetObject(c, input)
}
