/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
)

const ExportTopic string = "platform.export.requests"

// ExportCfg is the global variable containing the runtime configuration
var ExportCfg *ExportConfig

// ExportConfig represents the runtime configuration
type ExportConfig struct {
	Hostname        string
	PublicPort      int
	MetricsPort     int
	PrivatePort     int
	Logging         *loggingConfig
	LogLevel        string
	Debug           bool
	DBConfig        dbConfig
	KafkaConfig     kafkaConfig
	OpenAPIFilePath string
	Psks            []string

	Channels channels
}

type channels struct {
	ProducerMessagesChan chan *kafka.Message
	ToS3Chan             chan io.Reader
}

func (c channels) CloseChannels() {
	close(c.ProducerMessagesChan)
	close(c.ToS3Chan)
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
	Brokers      []string
	GroupID      string
	ExportsTopic string
	SSLConfig    kafkaSSLConfig
}

type kafkaSSLConfig struct {
	CA            string
	Username      string
	Password      string
	SASLMechanism string
	Protocol      string
}

// type StorageCfg struct {
// 	StorageBucket    string
// 	StorageEndpoint  string
// 	StorageAccessKey string
// 	StorageSecretKey string
// 	UseSSL           bool
// }

var config *ExportConfig

// initialize the configuration for service
func init() {
	options := viper.New()
	options.SetDefault("PublicPort", 8000)
	options.SetDefault("MetricsPort", 9000)
	options.SetDefault("PrivatePort", 10000)
	options.SetDefault("LogLevel", "INFO")
	options.SetDefault("Debug", false)
	options.SetDefault("OpenAPIFilePath", "./static/spec/openapi.json")
	options.SetDefault("psks", strings.Split(os.Getenv("EXPORTS_PSKS"), ","))

	// DB defaults
	options.SetDefault("Database", "pgsql")
	options.SetDefault("PGSQL_USER", "postgres")
	options.SetDefault("PGSQL_PASSWORD", "postgres")
	options.SetDefault("PGSQL_HOSTNAME", "localhost")
	options.SetDefault("PGSQL_PORT", "15433")
	options.SetDefault("PGSQL_DATABASE", "postgres")

	// kafka defaults
	options.SetDefault("KafakAnnounceTopic", ExportTopic)
	options.SetDefault("KafkaBrokers", strings.Split(os.Getenv("KAFKA_BROKERS"), ","))
	options.SetDefault("KafkaGroupID", "export")

	options.AutomaticEnv()

	if options.GetBool("Debug") {
		options.Set("LogLevel", "DEBUG")
	}

	kubenv := viper.New()
	kubenv.AutomaticEnv()

	config = &ExportConfig{
		Hostname:        kubenv.GetString("Hostname"),
		PublicPort:      options.GetInt("PublicPort"),
		MetricsPort:     options.GetInt("MetricsPort"),
		PrivatePort:     options.GetInt("PrivatePort"),
		Debug:           options.GetBool("Debug"),
		LogLevel:        options.GetString("LogLevel"),
		OpenAPIFilePath: options.GetString("OpenAPIFilePath"),
		Psks:            options.GetStringSlice("psks"),
	}

	config.Channels = channels{
		ProducerMessagesChan: make(chan *kafka.Message), // TODO: determine an appropriate buffer (if one is actually necessary)
		ToS3Chan:             make(chan io.Reader),      // TODO: determine an appropriate buffer (if one is actually necessary),
	}

	database := options.GetString("database")

	if database == "pgsql" {
		config.DBConfig = dbConfig{
			User:     options.GetString("PGSQL_USER"),
			Password: options.GetString("PGSQL_PASSWORD"),
			Hostname: options.GetString("PGSQL_HOSTNAME"),
			Port:     options.GetString("PGSQL_PORT"),
			Name:     options.GetString("PGSQL_DATABASE"),
			SSLCfg: dbSSLConfig{
				SSLMode: "prefer",
			},
		}
	}

	config.KafkaConfig = kafkaConfig{
		Brokers:      options.GetStringSlice("KafkaBrokers"),
		GroupID:      options.GetString("KafkaGroupID"),
		ExportsTopic: options.GetString("KafakAnnounceTopic"),
	}

	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig

		config.PublicPort = *cfg.PublicPort
		config.MetricsPort = cfg.MetricsPort
		config.PrivatePort = *cfg.PrivatePort

		config.DBConfig = dbConfig{
			User:     cfg.Database.Username,
			Password: cfg.Database.Password,
			Hostname: cfg.Database.Hostname,
			Port:     fmt.Sprint(cfg.Database.Port),
			Name:     cfg.Database.Name,
			SSLCfg: dbSSLConfig{
				SSLMode: cfg.Database.SslMode,
				RdsCa:   cfg.Database.RdsCa,
			},
		}

		config.KafkaConfig.Brokers = clowder.KafkaServers
		broker := cfg.Kafka.Brokers[0]
		if broker.Authtype != nil {
			caPath, err := cfg.KafkaCa(broker)
			if err != nil {
				panic("Kafka CA failed to write")
			}
			config.KafkaConfig.SSLConfig = kafkaSSLConfig{
				Username:      *broker.Sasl.Username,
				Password:      *broker.Sasl.Password,
				SASLMechanism: "SCRAM-SHA-512",
				Protocol:      "sasl_ssl",
				CA:            caPath,
			}
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
