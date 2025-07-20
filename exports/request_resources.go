/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package exports

import (
	"context"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"

	cloudEventSchema "github.com/RedHatInsights/event-schemas-go/apps/exportservice/v1"
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
				var filters map[string]interface{}

				if source.Filters != nil && len(source.Filters) > 0 {
					filters, err = ekafka.JsonToMap(source.Filters)
					if err != nil {
						log.Errorw("failed unmarshalling filters", "error", err)
					}
				}

				format, ok := ekafka.ParseFormat(string(payload.Format))
				if !ok {
					log.Errorw("failed parsing format", "error", err)
					// FIXME:
					// return err
					continue // Skip this source and continue with the next one
				}

				headers := ekafka.KafkaHeader{
					Application: source.Application,
					IDheader:    identity,
				}
				kpayload := ekafka.KafkaMessage{
					ID:          uuid.New(),
					Schema:      kafkaConfig.EventSchema,
					Source:      kafkaConfig.EventSource,
					Subject:     fmt.Sprintf("urn:redhat:subject:export-service:request:%s", payload.ID.String()),
					SpecVersion: kafkaConfig.EventSpecVersion,
					Type:        kafkaConfig.EventType,
					Time:        time.Now().UTC().Format(formatDateTime),
					OrgID:       payload.OrganizationID,
					DataSchema:  kafkaConfig.EventDataSchema,
					Data: cloudEventSchema.ResourceRequest{
						ResourceRequest: cloudEventSchema.ResourceRequestClass{
							Application:       source.Application,
							ExportRequestUUID: payload.ID.String(),
							Filters:           filters,
							Format:            format,
							Resource:          source.Resource,
							UUID:              source.ID.String(),
							XRhIdentity:       identity,
						},
					},
				}

				msg, err := kpayload.ToMessage(headers, kafkaConfig.ExportsTopic)
				if err != nil {
					log.Errorw("failed to create kafka message", "error", err)
					// FIXME:
					// return err
					continue // Skip this source and continue with the next one
				}

				log.Debug("sending kafka message to the producer")
				kafkaChan <- msg // TODO: what should we do if the message is never sent to the producer?
				log.Infof("sent kafka message to the producer: %+v", msg)
			}
		}()
	}
}
