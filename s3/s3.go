package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	econfig "github.com/redhatinsights/export-service-go/config"
)

const defaultRegion = "us-east-1"

func NewS3Client(cfg econfig.ExportConfig, log *zap.SugaredLogger) *s3.Client {
	scfg := cfg.StorageConfig

	// Check if the region is set in the environment
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = defaultRegion
	}

	// Create AWS config with credentials
	awsCfg := aws.Config{
		Region: region,
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     scfg.AccessKey,
				SecretAccessKey: scfg.SecretKey,
			}, nil
		}),
	}

	// Create S3 client with custom endpoint
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &scfg.Endpoint
		o.UsePathStyle = true
	})

	log.Infof("s3 client configured")
	return s3Client
}
