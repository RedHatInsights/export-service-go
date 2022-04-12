package config

import (
	"fmt"
	"os"

	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"

	"github.com/spf13/viper"
)

const (
	StatusTopic   string = "platform.export.status"
	AnnounceTopic string = "platform.export.announce"
)

// ExportConfig represents the runtime configuration
type ExportConfig struct {
	Hostname      string
	DBConfig      DBConfig
	KafkaConfig   KafkaCfg
	WebPort       int
	MetricsPort   int
	StorageConfig StorageCfg
	LoggingConfig LoggingCfg
}

type DBConfig struct {
	Type     string `json:"type,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"-"`
	Hostname string `json:"hostname,omitempty"`
	Port     string `json:"port,omitempty"`
	Name     string `json:"name,omitempty"`
}

type KafkaCfg struct {
	KafkaBrokers         []string
	KafkaGroupID         string
	KafkaStatusTopic     string
	KafkaDeliveryReports bool
	KafkaAnnounceTopic   string
	ValidTopics          []string
	KafkaSSLConfig       KafkaSSLCfg
}

type KafkaSSLCfg struct {
	KafkaCA       string
	KafkaUsername string
	KafkaPassword string
	SASLMechanism string
	Protocol      string
}

type StorageCfg struct {
	StageBucket      string
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	UseSSL           bool
}

type LoggingCfg struct {
	LogGroup           string
	LogLevel           string
	AwsRegion          string
	AwsAccessKeyId     string
	AwsSecretAccessKey string
}

var ExportCfg *ExportConfig

func init() {

	options := viper.New()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// global logging
	options.SetDefault("logLevel", "INFO")
	options.SetDefault("Hostname", hostname)

	// Kafka config
	options.SetDefault("KafkaGroupID", "export")
	options.SetDefault("KafkaDeliveryReports", true)
	options.SetDefault("KafkaStatusTopic", StatusTopic)
	options.SetDefault("KafakAnnounceTopic", AnnounceTopic)

	// Global defaults
	options.SetEnvPrefix("EXPORT")
	options.AutomaticEnv()
	kubenv := viper.New()
	kubenv.AutomaticEnv()

	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig

		// DB
		options.SetDefault("PSQLUser", cfg.Database.Username)
		options.SetDefault("PSQLPassword", cfg.Database.Password)
		options.SetDefault("PSQLHostname", cfg.Database.Hostname)
		options.SetDefault("PSQLPort", cfg.Database.Port)
		options.SetDefault("PSQLDatabase", cfg.Database.Name)

		sb := os.Getenv("INGRESS_STAGEBUCKET")
		bucket := clowder.ObjectBuckets[sb]
		broker := cfg.Kafka.Brokers[0]

		// Kafka
		options.SetDefault("KafkaBrokers", clowder.KafkaServers)
		options.SetDefault("KafkaStatusTopic", clowder.KafkaTopics[StatusTopic].Name)
		// Kafka SSL Config
		if broker.Authtype != nil {
			options.Set("KafkaUsername", *broker.Sasl.Username)
			options.Set("KafkaPassword", *broker.Sasl.Password)
			options.Set("SASLMechanism", "SCRAM-SHA-512")
			options.Set("Protocol", "sasl_ssl")
			caPath, err := cfg.KafkaCa(broker)
			if err != nil {
				panic("Kafka CA failed to write")
			}
			options.Set("KafkaCA", caPath)
		}
		// Ports
		options.SetDefault("WebPort", cfg.PublicPort)
		options.SetDefault("MetricsPort", cfg.MetricsPort)
		// Storage
		options.SetDefault("StageBucket", bucket.RequestedName)
		options.SetDefault("MinioEndpoint", fmt.Sprintf("%s:%d", cfg.ObjectStore.Hostname, cfg.ObjectStore.Port))
		options.SetDefault("MinioAccessKey", cfg.ObjectStore.Buckets[0].AccessKey)
		options.SetDefault("MinioSecretKey", cfg.ObjectStore.Buckets[0].SecretKey)
		options.SetDefault("UseSSL", cfg.ObjectStore.Tls)
		// Cloudwatch
		options.SetDefault("logGroup", cfg.Logging.Cloudwatch.LogGroup)
		options.SetDefault("AwsRegion", cfg.Logging.Cloudwatch.Region)
		options.SetDefault("AwsAccessKeyId", cfg.Logging.Cloudwatch.AccessKeyId)
		options.SetDefault("AwsSecretAccessKey", cfg.Logging.Cloudwatch.SecretAccessKey)
	} else {
		// DB
		options.SetDefault("PSQLUser", "postgres")
		options.SetDefault("PSQLPassword", "postgres")
		options.SetDefault("PSQLHostname", "localhost")
		options.SetDefault("PSQLPort", "15433")
		options.SetDefault("PSQLDatabase", "postgres")
		// Kafka
		defaultBrokers := os.Getenv("INGRESS_KAFKA_BROKERS")
		if len(defaultBrokers) == 0 {
			defaultBrokers = "kafka:29092"
		}
		options.SetDefault("KafkaBrokers", []string{defaultBrokers})
		options.SetDefault("KafkaStatusTopic", "platform.payload-status")
		// Ports
		options.SetDefault("WebPort", 8080)
		options.SetDefault("MetricsPort", 9000)
		// Storage
		options.SetDefault("StageBucket", "available")
		// Cloudwatch
		options.SetDefault("LogGroup", "platform-dev")
		options.SetDefault("AwsRegion", "us-east-1")
		options.SetDefault("UseSSL", false)
		options.SetDefault("AwsAccessKeyId", os.Getenv("CW_AWS_ACCESS_KEY_ID"))
		options.SetDefault("AwsSecretAccessKey", os.Getenv("CW_AWS_SECRET_ACCESS_KEY"))
	}

	ExportCfg = &ExportConfig{
		Hostname:    options.GetString("Hostname"),
		WebPort:     options.GetInt("WebPort"),
		MetricsPort: options.GetInt("MetricsPort"),
		DBConfig: DBConfig{
			Type:     "psql",
			User:     options.GetString("PSQLUser"),
			Password: options.GetString("PSQLPassword"),
			Hostname: options.GetString("PSQLHostname"),
			Port:     options.GetString("PSQLPort"),
			Name:     options.GetString("PSQLDatabase"),
		},
		KafkaConfig: KafkaCfg{
			KafkaBrokers:         options.GetStringSlice("KafkaBrokers"),
			KafkaGroupID:         options.GetString("KafkaGroupID"),
			KafkaStatusTopic:     options.GetString("KafkaStatusTopic"),
			KafkaDeliveryReports: options.GetBool("KafkaDeliveryReports"),
			KafkaAnnounceTopic:   options.GetString("KafakAnnounceTopic"),
		},
		StorageConfig: StorageCfg{
			StageBucket:      options.GetString("StageBucket"),
			StorageEndpoint:  options.GetString("MinioEndpoint"),
			StorageAccessKey: options.GetString("MinioAccessKey"),
			StorageSecretKey: options.GetString("MinioSecretKey"),
			UseSSL:           options.GetBool("UseSSL"),
		},
		LoggingConfig: LoggingCfg{
			LogGroup:           options.GetString("logGroup"),
			LogLevel:           options.GetString("logLevel"),
			AwsRegion:          options.GetString("AwsRegion"),
			AwsAccessKeyId:     options.GetString("AwsAccessKeyId"),
			AwsSecretAccessKey: options.GetString("AwsSecretAccessKey"),
		},
	}

	if options.IsSet("KafkaUsername") {
		ExportCfg.KafkaConfig.KafkaSSLConfig.KafkaUsername = options.GetString("KafkaUsername")
		ExportCfg.KafkaConfig.KafkaSSLConfig.KafkaPassword = options.GetString("KafkaPassword")
		ExportCfg.KafkaConfig.KafkaSSLConfig.SASLMechanism = options.GetString("SASLMechanism")
		ExportCfg.KafkaConfig.KafkaSSLConfig.Protocol = options.GetString("Protocol")
		ExportCfg.KafkaConfig.KafkaSSLConfig.KafkaCA = options.GetString("KafkaCA")
	}
}
