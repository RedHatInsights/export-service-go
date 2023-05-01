package kafka

import (
	"encoding/json"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"gorm.io/datatypes"
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
	ID          uuid.UUID        `json:"id"`
	Source      string           `json:"source"`
	Subject     string           `json:"subject"`
	SpecVersion string           `json:"specversion"`
	Type        string           `json:"type"`
	Time        string           `json:"time"`
	OrgID       string           `json:"orgid"`
	DataSchema  string           `json:"dataschema"`
	Data        KafkaMessageData `json:"data"`
}

// TODO: This should be pulled from event-schemas-go
type KafkaMessageData struct {
	// The application being requested
	Application string `json:"application"`
	// The filters to be applied to the data
	Filters map[string]interface{} `json:"filters,omitempty"`
	// The format of the data to be exported
	Format Format `json:"format"`
	// The resource to be exported
	Resource string `json:"resource"`
	// A unique identifier for the request
	UUID string `json:"uuid"`
	// The Base64-encoded JSON identity header of the user making the request
	XRhIdentity string `json:"x-rh-identity"`
}

// The format of the data to be exported
type Format string

const (
	CSV  Format = "csv"
	JSON Format = "json"
)

var (
	formatsMap = map[string]Format{
		"csv":  CSV,
		"json": JSON,
	}
)

func ParseFormat(s string) (Format, bool) {
	result, ok := formatsMap[s]
	return result, ok
}

func JsonToInterface(jsonData datatypes.JSON) (map[string]interface{}, error) {
	jsonBytes, err := jsonData.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
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
