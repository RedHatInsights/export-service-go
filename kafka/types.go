package kafka

import (
	"encoding/json"

	cloudEventSchema "github.com/RedHatInsights/event-schemas-go/apps/exportservice/v1"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
)

type KafkaHeader struct {
	Application string `json:"application"`
	IDheader    string `json:"x-rh-identity"`
}

// ToHeader converts the KafkaHeader into a confluent kafka
// header
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
	ID          uuid.UUID                        `json:"id"`
	Schema      string                           `json:"$schema"`
	Source      string                           `json:"source"`
	Subject     string                           `json:"subject"`
	SpecVersion string                           `json:"specversion"`
	Type        string                           `json:"type"`
	Time        string                           `json:"time"`
	OrgID       string                           `json:"redhatorgid"`
	DataSchema  string                           `json:"dataschema"`
	Data        cloudEventSchema.ResourceRequest `json:"data"`
}

func ParseFormat(s string) (result cloudEventSchema.Format, ok bool) {
	switch s {
	case "csv":
		return cloudEventSchema.CSV, true
	case "json":
		return cloudEventSchema.JSON, true
	default:
		return "", false
	}
}

func JsonToMap(jsonData []byte) (map[string]interface{}, error) {
	var resultMap map[string]interface{}
	err := json.Unmarshal(jsonData, &resultMap)
	if err != nil {
		return nil, err
	}
	return resultMap, nil
}

// ToMessage converts the KafkaMessage struct to a confluent kafka.Message
// ready to be sent through the kafka producer
func (km KafkaMessage) ToMessage(header KafkaHeader, topic string) (*kafka.Message, error) {
	val, err := json.Marshal(km)
	if err != nil {
		return nil, err
	}
	return &kafka.Message{
		Headers: header.ToHeader(),
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: []byte(val),
	}, nil
}
