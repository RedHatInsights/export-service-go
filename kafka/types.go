package kafka

import (
	"encoding/json"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/redhatinsights/export-service-go/config"
)

var kcfg = config.ExportCfg.KafkaConfig

type KafkaHeader struct {
	Application string `json:"application"`
	IDheader    string `json:"x-rh-identity"`
}

func (kh KafkaHeader) ToHeader() []kafka.Header {
	result := []kafka.Header{}
	result = append(result, kafka.Header{
		Key:   "application",
		Value: []byte(kh.Application),
	})
	result = append(result, kafka.Header{
		Key:   "x-rh-identity",
		Value: []byte(kh.IDheader),
	})
	return result
}

type KafkaMessage struct {
	ExportUUID   string `json:"export_uuid"`
	Application  string `json:"application"`
	Format       string `json:"format"`
	ResourceName string `json:"resource"`
	ResourceUUID string `json:"resource_uuid"`
	Filters      []byte `json:"filters"`
	IDHeader     string `json:"x-rh-identity"`
}

func (km KafkaMessage) ToMessage(header KafkaHeader) (*kafka.Message, error) {
	val, err := json.Marshal(km)
	if err != nil {
		return nil, err
	}
	return &kafka.Message{
		Headers:        header.ToHeader(),
		TopicPartition: kafka.TopicPartition{Topic: &kcfg.KafkaAnnounceTopic, Partition: kafka.PartitionAny},
		Value:          []byte(val),
	}, nil
}
