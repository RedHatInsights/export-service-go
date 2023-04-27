/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package exports

import (
	"context"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	ekafka "github.com/redhatinsights/export-service-go/kafka"
	"github.com/redhatinsights/export-service-go/models"
)

type RequestApplicationResources func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload)

func KafkaRequestApplicationResources(kafkaChan chan *kafka.Message) RequestApplicationResources {
	var kafkaConfig = config.Get().KafkaConfig
	// sendPayload converts the individual sources of a payload into
	// kafka messages which are then sent to the producer through the
	// `messagesChan`
	return func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
		go func() {
			sources, err := payload.GetSources()
			if err != nil {
				log.Errorw("failed unmarshalling sources", "error", err)
				// FIXME:
				// return err
				return
			}

			for _, source := range sources {
				headers := ekafka.KafkaHeader{
					Application: source.Application,
					IDheader:    identity,
				}
				kpayload := ekafka.KafkaMessage{
					ID:          uuid.New(),
					Source:      kafkaConfig.EventSource,
					Subject:     kafkaConfig.EventSubject,
					SpecVersion: kafkaConfig.EventSpecVersion,
					Type:        kafkaConfig.EventType,
					Time:        time.Now().Format(time.RFC3339),
					OrgID:       payload.OrganizationID,
					DataSchema:  kafkaConfig.EventDataSchema,
					Data: ekafka.KafkaMessageData{
						ExportUUID:   payload.ID,
						Application:  source.Application,
						Format:       string(payload.Format),
						ResourceName: source.Resource,
						ResourceUUID: source.ID,
						Filters:      source.Filters,
					},
				}

				msg, err := kpayload.ToMessage(headers, kafkaConfig.ExportsTopic)
				if err != nil {
					log.Errorw("failed to create kafka message", "error", err)
					// FIXME:
					// return err
					return
				}

				log.Debug("sending kafka message to the producer")
				kafkaChan <- msg // TODO: what should we do if the message is never sent to the producer?
				log.Infof("sent kafka message to the producer: %+v", msg)
			}
		}()
	}
}
