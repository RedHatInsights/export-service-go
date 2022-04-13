// Copyright Red Hat

package logger

import (
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/redhatinsights/export-service-go/config"
	lc "github.com/redhatinsights/platform-go-middlewares/logging/cloudwatch"
	log "github.com/sirupsen/logrus"

	"github.com/redhatinsights/export-service-go/config"
)

// Log is an instance of the global logrus.Logger
var logLevel log.Level

var cfg *config.ExportConfig

// init initializes the API logger upon import
func init() {

	cfg = config.ExportCfg

	switch cfg.LoggingConfig.LogLevel {
	case "DEBUG":
		logLevel = log.DebugLevel
	case "ERROR":
		logLevel = log.ErrorLevel
	default:
		logLevel = log.InfoLevel
	}

	if cfg.LoggingConfig != nil && cfg.LoggingConfig.Region != "" {
		cred := credentials.NewStaticCredentials(cfg.LoggingConfig.AccessKeyID, cfg.LoggingConfig.SecretAccessKey, "")
		awsconf := aws.NewConfig().WithRegion(cfg.LoggingConfig.Region).WithCredentials(cred)
		hook, err := lc.NewBatchingHook(cfg.LoggingConfig.LogGroup, cfg.Hostname, awsconf, 10*time.Second)
		if err != nil {
			log.Info(err)
		}
		log.AddHook(hook)
		log.SetFormatter(&log.JSONFormatter{
			TimestampFormat: time.Now().Format("2006-01-02T15:04:05.999Z"),
			FieldMap: log.FieldMap{
				log.FieldKeyTime: "@timestamp",
			},
		})
	}

	log.SetOutput(os.Stdout)
	log.SetLevel(logLevel)
}
