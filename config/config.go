package config

import (
	"fmt"

	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
)

const (
	StatusTopic   string = "platform.export.status"
	AnnounceTopic string = "platform.export.announce"
)

var ExportCfg *ExportConfig

// ExportConfig represents the runtime configuration
type ExportConfig struct {
	Hostname        string
	Auth            bool
	WebPort         int
	MetricsPort     int
	Logging         *loggingConfig
	LogLevel        string
	Debug           bool
	DBConfig        dbConfig
	OpenAPIFilePath string
}

type dbConfig struct {
	User     string
	Password string
	Hostname string
	Port     string
	Name     string
}

type loggingConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	LogGroup        string
	Region          string
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
	StorageBucket    string
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	UseSSL           bool
}

var config *ExportConfig

// initialize the configuration for service
func init() {
	options := viper.New()
	options.SetDefault("WebPort", 8000)
	options.SetDefault("MetricsPort", 9000)
	options.SetDefault("LogLevel", "INFO")
	options.SetDefault("Auth", true)
	options.SetDefault("Debug", false)
	options.SetDefault("OpenAPIFilePath", "./static/spec/openapi.json")

	// DB defaults
	options.SetDefault("Database", "pgsql")
	options.SetDefault("PGSQL_USER", "postgres")
	options.SetDefault("PGSQL_PASSWORD", "postgres")
	options.SetDefault("PGSQL_HOSTNAME", "localhost")
	options.SetDefault("PGSQL_PORT", "15433")
	options.SetDefault("PGSQL_DATABASE", "postgres")

	options.AutomaticEnv()

	if options.GetBool("Debug") {
		options.Set("LogLevel", "DEBUG")
	}

	kubenv := viper.New()
	kubenv.AutomaticEnv()

	config = &ExportConfig{
		Hostname:        kubenv.GetString("Hostname"),
		Auth:            options.GetBool("Auth"),
		WebPort:         options.GetInt("WebPort"),
		MetricsPort:     options.GetInt("MetricsPort"),
		Debug:           options.GetBool("Debug"),
		LogLevel:        options.GetString("LogLevel"),
		OpenAPIFilePath: options.GetString("OpenAPIFilePath"),
	}

	database := options.GetString("database")

	if database == "pgsql" {
		config.DBConfig = dbConfig{
			User:     options.GetString("PGSQL_USER"),
			Password: options.GetString("PGSQL_PASSWORD"),
			Hostname: options.GetString("PGSQL_HOSTNAME"),
			Port:     options.GetString("PGSQL_PORT"),
			Name:     options.GetString("PGSQL_DATABASE"),
		}
	}

	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig

		config.WebPort = *cfg.PublicPort
		config.MetricsPort = cfg.MetricsPort

		config.DBConfig = dbConfig{
			User:     cfg.Database.Username,
			Password: cfg.Database.Password,
			Hostname: cfg.Database.Hostname,
			Port:     fmt.Sprint(cfg.Database.Port),
			Name:     cfg.Database.Name,
		}

		config.Logging = &loggingConfig{
			AccessKeyID:     cfg.Logging.Cloudwatch.AccessKeyId,
			SecretAccessKey: cfg.Logging.Cloudwatch.SecretAccessKey,
			LogGroup:        cfg.Logging.Cloudwatch.LogGroup,
			Region:          cfg.Logging.Cloudwatch.Region,
		}
	}

	ExportCfg = config
}
