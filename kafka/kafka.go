package kafka

import (
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"
)

var (
	cfg     = config.ExportCfg
	log     = logger.Log
	msgChan = cfg.Channels.ProducerMessagesChan

	messagesPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ingress_kafka_produced",
		Help: "Number of messages produced to kafka",
	}, []string{"topic"})
	messagePublishElapsed = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "ingress_publish_seconds",
		Help: "Number of seconds spent writing kafka messages",
	}, []string{"topic"})
	publishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ingress_kafka_produce_failures",
		Help: "Number of times a message was failed to be produced",
	}, []string{"topic"})
	producerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ingress_kafka_producer_go_routine_count",
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
func (p *Producer) StartProducer() {
	log.Infof("started kafka producer: %+v", p)
	topic := cfg.KafkaConfig.ExportsTopic
	for msg := range msgChan {
		go func(msg *kafka.Message) {
			producerCount.Inc()
			defer producerCount.Dec()
			start := time.Now()

			if err := p.Produce(msg, nil); err != nil { // pass nil chan so that delivery reports go to the Events() channel
				log.Errorw("failed to produce message", "error", err)
				return
			}

			messagePublishElapsed.With(prometheus.Labels{"topic": topic}).Observe(time.Since(start).Seconds())

			// Delivery report handler for produced messages
			for e := range p.Events() {
				switch ev := e.(type) {
				case *kafka.Message:
					if ev.TopicPartition.Error != nil {
						log.Errorw("error publishing to kafka", "error", ev.TopicPartition.Error)
						msgChan <- msg
						publishFailures.With(prometheus.Labels{"topic": topic}).Inc()
					} else {
						log.Infof("delivered message to %v", ev.TopicPartition)
						messagesPublished.With(prometheus.Labels{"topic": topic}).Inc()
					}
				}
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
		kcfg = &kafka.ConfigMap{
			"bootstrap.servers": brokers,
			"client.id":         cfg.Hostname,
			"security.protocol": ssl.Protocol,
			"sasl.mechanism":    ssl.SASLMechanism,
			"ssl.ca.location":   ssl.CA,
			"sasl.username":     ssl.Username,
			"sasl.password":     ssl.Password,
		}
	}

	p, err := kafka.NewProducer(kcfg)
	return &Producer{p}, err
}
