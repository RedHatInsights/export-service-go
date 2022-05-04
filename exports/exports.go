/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/errors"
	ekafka "github.com/redhatinsights/export-service-go/kafka"
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
)

var log = logger.Log
var messagesChan = config.ExportCfg.Channels.ProducerMessagesChan

type Export struct {
	Cfg *config.ExportConfig
	DB  models.DBInterface
	Log *zap.SugaredLogger
}

// ExportRouter is a router for all of the external routes for the /exports endpoint.
func (e *Export) ExportRouter(r chi.Router) {
	r.Post("/", e.PostExport)
	r.With(middleware.PaginationCtx).Get("/", e.ListExports)
	r.Route("/{exportUUID}", func(sub chi.Router) {
		sub.With(middleware.GZIPContentType).Get("/", e.GetExport) // TODO: will this middleware work correctly?
		sub.Delete("/", e.DeleteExport)
		sub.Get("/status", e.GetExportStatus)
	})
}

// PostExport handles POST requests to the /exports endpoint.
func (e *Export) PostExport(w http.ResponseWriter, r *http.Request) {
	reqID := request_id.GetReqID(r.Context())
	user := middleware.GetUserIdentity(r.Context())

	var payload models.ExportPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		errors.BadRequestError(w, err.Error())
		return
	}
	payload.RequestID = reqID
	payload.User = user
	export, err := e.DB.APICreate(&payload)
	if err != nil {
		log.Errorw("error creating payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if err := json.NewEncoder(w).Encode(&export); err != nil {
		log.Errorw("error while trying to encode", "error", err)
		errors.InternalServerError(w, err.Error())
	}

	// send the payload to the producer with a goroutine so
	// that we do not block the response
	go sendPayload(payload, r)
}

// sendPayload converts the individual sources of a payload into
// kafka messages which are then sent to the producer through the
// `messagesChan`
func sendPayload(payload models.ExportPayload, r *http.Request) {
	headers := ekafka.KafkaHeader{
		Application: payload.Application,
		IDheader:    r.Header["X-Rh-Identity"][0],
	}
	sources, err := payload.GetSources()
	if err != nil {
		log.Errorw("failed unmarshalling sources", "error", err)
		return
	}
	for _, source := range sources {
		kpayload := ekafka.KafkaMessage{
			ExportUUID:   payload.ID,
			Application:  payload.Application,
			Format:       string(payload.Format),
			ResourceName: source.Resource,
			ResourceUUID: source.ID,
			Filters:      source.Filters,
			IDHeader:     r.Header["X-Rh-Identity"][0],
		}
		msg, err := kpayload.ToMessage(headers)
		if err != nil {
			log.Errorw("failed to create kafka message", "error", err)
			return
		}
		log.Debug("sending kafka message to the producer")
		messagesChan <- msg // TODO: what should we do if the message is never sent to the producer?
		log.Infof("sent kafka message to the producer: %+v", msg)
	}
}

func buildQuery(q url.Values) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	for k, v := range q {
		if len(v) > 1 {
			return nil, fmt.Errorf("query param `%s` has too many search values", k)
		}
		result[k] = v[0]
	}

	return result, nil
}

// ListExports handle GET requests to the /exports endpoint.
func (e *Export) ListExports(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserIdentity(r.Context())
	page := middleware.GetPagination(r.Context())

	q := r.URL.Query()
	// offset/limit are for pagination so remove them so they are not inserted into the db query
	q.Del("offset")
	q.Del("limit")
	// query, err := buildQuery(q)
	// if err != nil {
	// 	errors.BadRequestError(w, err.Error())
	// 	return
	// }

	// exports := []*APIExport{}
	// result := db.DB.Model(
	// 	&models.ExportPayload{}).Where(
	// 	&models.ExportPayload{User: user}).Where(
	// 	query).Find(
	// 	&exports)
	exports, err := e.DB.APIList(user)
	if err != nil {
		errors.InternalServerError(w, err)
		return
	}
	resp, err := middleware.GetPaginatedResponse(r.URL, page, exports)
	if err != nil {
		log.Errorw("error while paginating data", "error", err)
		errors.InternalServerError(w, err)
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}

// GetExport handles GET requests to the /exports/{exportUUID} endpoint.
// This function is responsible for returning the S3 object.
func (e *Export) GetExport(w http.ResponseWriter, r *http.Request) {
	// func responsible for downloading from s3
	errors.NotImplementedError(w)
}

// DeleteExport handles DELETE requests to the /exports/{exportUUID} endpoint.
func (e *Export) DeleteExport(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return
	}

	user := middleware.GetUserIdentity(r.Context())

	rowsAffected, err := e.DB.Delete(exportUUID, user)
	if err != nil {
		log.Errorw("error deleting payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if rowsAffected == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
		return
	}

}

// GetExportStatus handles GET requests to the /exports/{exportUUID}/status endpoint.
func (e *Export) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return
	}

	user := middleware.GetUserIdentity(r.Context())

	export, err := e.DB.APIGetWithUser(exportUUID, user)
	if err != nil {
		log.Errorw("error querying for payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if export == nil {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
		return
	}
	if err := json.NewEncoder(w).Encode(&export); err != nil {
		log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}
