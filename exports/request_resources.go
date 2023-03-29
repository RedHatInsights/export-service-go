/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package exports

import (
	"context"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"

	ekafka "github.com/redhatinsights/export-service-go/kafka"
	"github.com/redhatinsights/export-service-go/models"
)

type RequestApplicationResources func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload)

func KafkaRequestApplicationResources(kafkaChan chan *kafka.Message) RequestApplicationResources {
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
					ExportUUID:   payload.ID,
					Format:       string(payload.Format),
					Application:  source.Application,
					ResourceName: source.Resource,
					ResourceUUID: source.ID,
					Filters:      source.Filters,
					IDHeader:     identity,
				}

				msg, err := kpayload.ToMessage(headers)
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
