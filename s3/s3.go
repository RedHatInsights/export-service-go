package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	econfig "github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"
)

var Client *s3.Client
var cfg = econfig.ExportCfg
var log = logger.Log

type S3CreateMultipartUploadAPI interface {
	CreateMultipartUpload(ctx context.Context,
		params *s3.CreateMultipartUploadInput,
		optFns ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
}

// S3PutObjectAPI defines the interface for the PutObject function.
// We use this interface to test the function using a mocked service.
type S3PutObjectAPI interface {
	PutObject(ctx context.Context,
		params *s3.PutObjectInput,
		optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3PartUploaderAPI defines the interface for the PartUploader function.
// We use this interface to test the function using a mocked service.
type S3PartUploaderAPI interface {
	UploadPart(ctx context.Context,
		params *s3.UploadPartInput,
		optFns ...func(*s3.Options)) (*s3.UploadPartOutput, error)
}

func CreateMultipart(c context.Context, api S3CreateMultipartUploadAPI, input *s3.CreateMultipartUploadInput) (*s3.CreateMultipartUploadOutput, error) {
	return api.CreateMultipartUpload(c, input)
}

// UploadFile uploads a file to an Amazon Simple Storage Service (Amazon S3) bucket
// Inputs:
//     c is the context of the method call, which includes the AWS Region
//     api is the interface that defines the method call
//     input defines the input arguments to the service call.
// Output:
//     If success, a UploadPartOutput object containing the result of the service call and nil
//     Otherwise, nil and an error from the call to PartUploader
func UploadFile(c context.Context, api S3PartUploaderAPI, input *s3.UploadPartInput) (*s3.UploadPartOutput, error) {
	return api.UploadPart(c, input)
}

func init() {
	scfg := cfg.StorageConfig

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if cfg.Debug {
			return aws.Endpoint{
				URL:               scfg.Endpoint,
				HostnameImmutable: true,
			}, nil
		}

		// returning EndpointNotFoundError will allow the service to fallback to it's default resolution
		return aws.Endpoint{
			URL: scfg.Endpoint,
		}, &aws.EndpointNotFoundError{}
	})

	creds := aws.CredentialsProviderFunc(func(c context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     scfg.AccessKey,
			SecretAccessKey: scfg.SecretKey,
		}, nil
	})

	s3cfg := aws.Config{
		Region:                      "us-east-1",
		Credentials:                 creds,
		EndpointResolverWithOptions: resolver,
	}

	// s3cfg, err := config.LoadDefaultConfig(context.Background(), config.WithEndpointResolverWithOptions(customResolver))
	// if err != nil {
	// 	log.Panic(err)
	// }

	Client = s3.NewFromConfig(s3cfg)
	log.Infof("s3 client configured")
}
