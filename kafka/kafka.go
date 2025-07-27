package kafka

import (
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"
)

var (
	cfg = config.Get()
	log = logger.Get()

	messagesPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "export_service_kafka_produced",
		Help: "Number of messages produced to kafka",
	}, []string{"topic"})
	messagePublishElapsed = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "export_service_publish_seconds",
		Help: "Number of seconds spent writing kafka messages",
	}, []string{"topic"})
	publishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "export_service_kafka_produce_failures",
		Help: "Number of times a message was failed to be produced",
	}, []string{"topic"})
	producerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "export_service_kafka_producer_go_routine_count",
		Help: "Number of go routines currently publishing to kafka",
	})
)

func init() {
	prometheus.MustRegister(messagesPublished)
	prometheus.MustRegister(messagePublishElapsed)
	prometheus.MustRegister(publishFailures)
	prometheus.MustRegister(producerCount)
}

type Producer struct{ *kafka.Producer }

// StartProducer produces kafka messages on the kafka topic
func (p *Producer) StartProducer(msgChan chan *kafka.Message) {
	log.Infof("started kafka producer: %+v", p)
	topic := cfg.KafkaConfig.ExportsTopic
	for msg := range msgChan {
		go func(msg *kafka.Message) {
			deliveryChan := make(chan kafka.Event)
			defer close(deliveryChan)

			producerCount.Inc()
			defer producerCount.Dec()

			start := time.Now()
			err := p.Produce(msg, deliveryChan)

			if err != nil {
				log.Errorw("failed to produce message", "error", err)
				return
			}

			messagePublishElapsed.With(prometheus.Labels{"topic": topic}).Observe(time.Since(start).Seconds())

			e := <-deliveryChan
			m, ok := e.(*kafka.Message)
			if !ok {
				log.Errorw("error publishing to kafka", "error", "invalid message type")
				return
			}

			if m.TopicPartition.Error != nil {
				log.Errorw("error publishing to kafka", "error", m.TopicPartition.Error)
				msgChan <- msg
				publishFailures.With(prometheus.Labels{"topic": topic}).Inc()
			} else {
				messagesPublished.With(prometheus.Labels{"topic": topic}).Inc()
			}
		}(msg)
	}
}

// NewProducer generates a new kafka producer
func NewProducer() (*Producer, error) {
	brokers := strings.Join(cfg.KafkaConfig.Brokers, ",")
	log.Infow("kakfa configuration values",
		"client.id", cfg.Hostname,
		"bootstrap.servers", brokers,
		"topic", cfg.KafkaConfig.ExportsTopic,
		"loglevel", cfg.LogLevel,
		"debug", cfg.Debug,
	)

	kcfg := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"client.id":         cfg.Hostname,
	}

	if cfg.KafkaConfig.SSLConfig.SASLMechanism != "" {
		ssl := cfg.KafkaConfig.SSLConfig

		_ = kcfg.SetKey("security.protocol", ssl.Protocol)
		_ = kcfg.SetKey("sasl.mechanism", ssl.SASLMechanism)
		_ = kcfg.SetKey("sasl.username", ssl.Username)
		_ = kcfg.SetKey("sasl.password", ssl.Password)
	}

	if cfg.KafkaConfig.SSLConfig.CA != "" {
		_ = kcfg.SetKey("ssl.ca.location", cfg.KafkaConfig.SSLConfig.CA)
	}

	p, err := kafka.NewProducer(kcfg)
	return &Producer{p}, err
}
