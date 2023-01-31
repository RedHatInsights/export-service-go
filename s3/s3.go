package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	econfig "github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"
)

var (
	Client *s3.Client
	cfg    = econfig.ExportCfg
	log    = logger.Log
)

const defaultRegion = "us-east-1"

func init() {
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

	Client = s3.NewFromConfig(s3cfg)
	log.Infof("s3 client configured")
}
