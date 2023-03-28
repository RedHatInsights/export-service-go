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

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               scfg.Endpoint,
			SigningRegion:     defaultRegion,
			HostnameImmutable: true,
		}, nil
	})

	creds := aws.CredentialsProviderFunc(func(c context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     scfg.AccessKey,
			SecretAccessKey: scfg.SecretKey,
		}, nil
	})

	s3cfg := aws.Config{
		Region:                      defaultRegion,
		Credentials:                 creds,
		EndpointResolverWithOptions: resolver,
	}

	log.Infof("s3 client configured")
	return s3.NewFromConfig(s3cfg)
}
