/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
)

const ExportTopic string = "platform.export.requests"

// ExportConfig represents the runtime configuration
type ExportConfig struct {
	Hostname                      string
	PublicPort                    int
	PublicHttpServerReadTimeout   time.Duration
	PublicHttpServerWriteTimeout  time.Duration
	PrivateHttpServerReadTimeout  time.Duration
	PrivateHttpServerWriteTimeout time.Duration
	MetricsPort                   int
	PrivatePort                   int
	Logging                       *loggingConfig
	LogLevel                      string
	Debug                         bool
	DBConfig                      dbConfig
	StorageConfig                 storageConfig
	KafkaConfig                   kafkaConfig
	RateLimitConfig               rateLimitConfig
	OpenAPIPrivatePath            string
	OpenAPIPublicPath             string
	Psks                          []string
	ExportExpiryDays              int
	ExportableApplications        map[string]map[string]bool
	MaxPayloadSize                int
}

type dbConfig struct {
	User     string
	Password string
	Hostname string
	Port     string
	Name     string
	SSLCfg   dbSSLConfig
}

type dbSSLConfig struct {
	RdsCa   *string
	SSLMode string
}

type loggingConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	LogGroup        string
	Region          string
}

type kafkaConfig struct {
	Brokers          []string
	GroupID          string
	ExportsTopic     string
	SSLConfig        kafkaSSLConfig
	EventSource      string
	EventSpecVersion string
	EventType        string
	EventDataSchema  string
	EventSchema      string
}

type kafkaSSLConfig struct {
	CA            string
	Username      string
	Password      string
	SASLMechanism string
	Protocol      string
}

type storageConfig struct {
	Bucket                  string
	Endpoint                string
	AccessKey               string
	SecretKey               string
	UseSSL                  bool
	AwsUploaderBufferSize   int64
	AwsDownloaderBufferSize int64
}

type rateLimitConfig struct {
	Rate  int
	Burst int
}

var (
	config *ExportConfig
	doOnce sync.Once
)

// initialize the configuration for service
func Get() *ExportConfig {
	doOnce.Do(func() {
		options := viper.New()
		options.SetDefault("PUBLIC_PORT", 8000)
		options.SetDefault("METRICS_PORT", 9000)
		options.SetDefault("PRIVATE_PORT", 10000)
		options.SetDefault("PUBLIC_HTTP_SERVER_READ_TIMEOUT", 5*time.Second)
		options.SetDefault("PUBLIC_HTTP_SERVER_WRITE_TIMEOUT", 10*time.Second)
		options.SetDefault("PRIVATE_HTTP_SERVER_READ_TIMEOUT", 5*time.Second)
		options.SetDefault("PRIVATE_HTTP_SERVER_WRITE_TIMEOUT", 10*time.Second)
		options.SetDefault("LOG_LEVEL", "INFO")
		options.SetDefault("DEBUG", false)
		options.SetDefault("OPEN_API_FILE_PATH", "./static/spec/openapi.json")
		options.SetDefault("OPEN_API_PRIVATE_PATH", "./static/spec/private.json")
		options.SetDefault("PSKS", strings.Split(os.Getenv("EXPORTS_PSKS"), ","))
		options.SetDefault("EXPORT_EXPIRY_DAYS", 7)
		options.SetDefault("EXPORT_ENABLE_APPS", "{\"exampleApp\":[\"exampleResource\", \"anotherExampleResource\"]}")
		options.SetDefault("MAX_PAYLOAD_SIZE", 500)

		// DB defaults
		options.SetDefault("PGSQL_USER", "postgres")
		options.SetDefault("PGSQL_PASSWORD", "postgres")
		options.SetDefault("PGSQL_HOSTNAME", "localhost")
		options.SetDefault("PGSQL_PORT", "15433")
		options.SetDefault("PGSQL_DATABASE", "postgres")

		// Minio defaults
		options.SetDefault("MINIO_HOST", "localhost")
		options.SetDefault("MINIO_PORT", "9099")
		options.SetDefault("MINIO_SSL", false)

		// Kafka defaults
		options.SetDefault("KAFKA_ANNOUNCE_TOPIC", ExportTopic)
		options.SetDefault("KAFKA_BROKERS", strings.Split(os.Getenv("KAFKA_BROKERS"), ","))
		options.SetDefault("KAFKA_GROUP_ID", "export")
		options.SetDefault("KAFKA_EVENT_SOURCE", "urn:redhat:source:console:app:export-service")
		options.SetDefault("KAFKA_EVENT_SPECVERSION", "1.0")
		options.SetDefault("KAFKA_EVENT_TYPE", "com.redhat.console.export-service.request")
		options.SetDefault("KAFKA_EVENT_DATASCHEMA", "https://console.redhat.com/api/schemas/apps/export-service/v1/resource-request.json")
		options.SetDefault("KAFKA_EVENT_SCHEMA", "https://console.redhat.com/api/schemas/events/v1/events.json")

		options.SetDefault("AWS_UPLOADER_BUFFER_SIZE", 10*1024*1024)
		options.SetDefault("AWS_DOWNLOADER_BUFFER_SIZE", 10*1024*1024)

		// Rate limit defaults
		options.SetDefault("RATE_LIMIT_RATE", 100)
		options.SetDefault("RATE_LIMIT_BURST", 60)

		options.AutomaticEnv()

		kubenv := viper.New()
		kubenv.AutomaticEnv()

		config = &ExportConfig{
			Hostname:                      kubenv.GetString("Hostname"),
			PublicPort:                    options.GetInt("PUBLIC_PORT"),
			MetricsPort:                   options.GetInt("METRICS_PORT"),
			PrivatePort:                   options.GetInt("PRIVATE_PORT"),
			PublicHttpServerReadTimeout:   options.GetDuration("PUBLIC_HTTP_SERVER_READ_TIMEOUT"),
			PublicHttpServerWriteTimeout:  options.GetDuration("PUBLIC_HTTP_SERVER_WRITE_TIMEOUT"),
			PrivateHttpServerReadTimeout:  options.GetDuration("PRIVATE_HTTP_SERVER_READ_TIMEOUT"),
			PrivateHttpServerWriteTimeout: options.GetDuration("PRIVATE_HTTP_SERVER_WRITE_TIMEOUT"),
			Debug:                         options.GetBool("DEBUG"),
			LogLevel:                      options.GetString("LOG_LEVEL"),
			OpenAPIPublicPath:             options.GetString("OPEN_API_FILE_PATH"),
			OpenAPIPrivatePath:            options.GetString("OPEN_API_PRIVATE_PATH"),
			Psks:                          options.GetStringSlice("PSKS"),
			ExportExpiryDays:              options.GetInt("EXPORT_EXPIRY_DAYS"),
			ExportableApplications:        convertExportableAppsFromConfigToInternal(options.GetStringMapStringSlice("EXPORT_ENABLE_APPS")),
			MaxPayloadSize:                options.GetInt("MAX_PAYLOAD_SIZE"),
		}

		config.DBConfig = dbConfig{
			User:     options.GetString("PGSQL_USER"),
			Password: options.GetString("PGSQL_PASSWORD"),
			Hostname: options.GetString("PGSQL_HOSTNAME"),
			Port:     options.GetString("PGSQL_PORT"),
			Name:     options.GetString("PGSQL_DATABASE"),
			SSLCfg: dbSSLConfig{
				SSLMode: "disable",
			},
		}

		config.Logging = &loggingConfig{}

		config.StorageConfig = storageConfig{
			Bucket:                  "exports-bucket",
			Endpoint:                buildBaseHttpUrl(options.GetBool("MINIO_SSL"), options.GetString("MINIO_HOST"), options.GetInt("MINIO_PORT")),
			AccessKey:               options.GetString("AWS_ACCESS_KEY"),
			SecretKey:               options.GetString("AWS_SECRET_ACCESS_KEY"),
			UseSSL:                  options.GetBool("MINIO_SSL"),
			AwsUploaderBufferSize:   options.GetInt64("AWS_UPLOADER_BUFFER_SIZE"),
			AwsDownloaderBufferSize: options.GetInt64("AWS_DOWNLOADER_BUFFER_SIZE"),
		}

		config.KafkaConfig = kafkaConfig{
			Brokers:          options.GetStringSlice("KAFKA_BROKERS"),
			GroupID:          options.GetString("KAFKA_GROUP_ID"),
			ExportsTopic:     options.GetString("KAFKA_ANNOUNCE_TOPIC"),
			EventSource:      options.GetString("KAFKA_EVENT_SOURCE"),
			EventSpecVersion: options.GetString("KAFKA_EVENT_SPECVERSION"),
			EventType:        options.GetString("KAFKA_EVENT_TYPE"),
			EventDataSchema:  options.GetString("KAFKA_EVENT_DATASCHEMA"),
			EventSchema:      options.GetString("KAFKA_EVENT_SCHEMA"),
		}

		config.RateLimitConfig = rateLimitConfig{
			Rate:  options.GetInt("RATE_LIMIT_RATE"),
			Burst: options.GetInt("RATE_LIMIT_BURST"),
		}

		if clowder.IsClowderEnabled() {
			cfg := clowder.LoadedConfig

			config.PublicPort = *cfg.PublicPort
			config.MetricsPort = cfg.MetricsPort
			config.PrivatePort = *cfg.PrivatePort

			exportBucket := options.GetString("EXPORT_SERVICE_BUCKET")
			exportBucketInfo := clowder.ObjectBuckets[exportBucket]

			rdsCaPath, err := getRdsCaPath(cfg)
			if err != nil {
				panic("RDS CA failed to write: " + err.Error())
			}

			config.DBConfig.User = cfg.Database.Username
			config.DBConfig.Password = cfg.Database.Password
			config.DBConfig.Hostname = cfg.Database.Hostname
			config.DBConfig.Port = fmt.Sprint(cfg.Database.Port)
			config.DBConfig.Name = cfg.Database.Name
			config.DBConfig.SSLCfg.SSLMode = cfg.Database.SslMode
			config.DBConfig.SSLCfg.RdsCa = rdsCaPath

			config.KafkaConfig.Brokers = clowder.KafkaServers
			config.KafkaConfig.ExportsTopic = clowder.KafkaTopics[ExportTopic].Name
			if config.KafkaConfig.ExportsTopic == "" {
				fmt.Println("WARNING: Export requests kafka topic is not set within Clowder!")
			}

			config.KafkaConfig.SSLConfig = kafkaSSLConfig{}

			broker := cfg.Kafka.Brokers[0]

			if broker.Cacert != nil {
				caPath, err := cfg.KafkaCa(broker)
				if err != nil {
					panic("Kafka CA failed to write")
				}
				config.KafkaConfig.SSLConfig.CA = caPath
			}

			if broker.Authtype != nil {
				securityProtocol := "sasl_ssl"
				if broker.SecurityProtocol != nil {
					securityProtocol = *broker.SecurityProtocol
				}

				config.KafkaConfig.SSLConfig.Username = *broker.Sasl.Username
				config.KafkaConfig.SSLConfig.Password = *broker.Sasl.Password
				config.KafkaConfig.SSLConfig.SASLMechanism = *broker.Sasl.SaslMechanism
				config.KafkaConfig.SSLConfig.Protocol = securityProtocol
			}

			config.Logging.AccessKeyID = cfg.Logging.Cloudwatch.AccessKeyId
			config.Logging.SecretAccessKey = cfg.Logging.Cloudwatch.SecretAccessKey
			config.Logging.LogGroup = cfg.Logging.Cloudwatch.LogGroup
			config.Logging.Region = cfg.Logging.Cloudwatch.Region

			bucket := cfg.ObjectStore.Buckets[0]
			config.StorageConfig.Bucket = exportBucketInfo.RequestedName
			config.StorageConfig.Endpoint = buildBaseHttpUrl(cfg.ObjectStore.Tls, cfg.ObjectStore.Hostname, cfg.ObjectStore.Port)
			config.StorageConfig.AccessKey = *bucket.AccessKey
			config.StorageConfig.SecretKey = *bucket.SecretKey
			config.StorageConfig.UseSSL = cfg.ObjectStore.Tls
		}
	})

	return config
}

func getRdsCaPath(cfg *clowder.AppConfig) (*string, error) {
	var rdsCaPath *string

	if cfg.Database.RdsCa != nil {
		rdsCaPathValue, err := cfg.RdsCa()
		if err != nil {
			return nil, err
		}
		rdsCaPath = &rdsCaPathValue
	}

	return rdsCaPath, nil
}

func buildBaseHttpUrl(tlsEnabled bool, hostname string, port int) string {
	var protocol string = "http"
	if tlsEnabled {
		protocol = "https"
	}

	return fmt.Sprintf("%s://%s:%d", protocol, hostname, port)
}

func convertExportableAppsFromConfigToInternal(config map[string][]string) map[string]map[string]bool {
	exportableApps := make(map[string]map[string]bool)

	for app, resources := range config {
		if _, ok := exportableApps[app]; !ok {
			exportableApps[app] = make(map[string]bool)
		}

		for _, resource := range resources {
			exportableApps[app][resource] = true
		}
	}

	return exportableApps
}
