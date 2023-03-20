/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package exports

import (
	"encoding/json"
	"fmt"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	"github.com/redhatinsights/export-service-go/s3"
)

// Internal contains the configuration and
type Internal struct {
	Cfg        *config.ExportConfig
	Compressor s3.StorageHandler
	DB         models.DBInterface
	Log        *zap.SugaredLogger
}

// InternalRouter is a router for all of the internal routes which require exportuuid,
// application name, and resourceuuid.
func (i *Internal) InternalRouter(r chi.Router) {
	r.Route("/{exportUUID}/{application}/{resourceUUID}", func(sub chi.Router) {
		sub.Use(middleware.URLParamsCtx)
		sub.Post("/upload", i.PostUpload)
		sub.Post("/error", i.PostError)
	})
}

// PostError receives a POST request from the export source which contains the
// errors associated with creating the export for the given resource.
func (i *Internal) PostError(w http.ResponseWriter, r *http.Request) {
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		InternalServerError(w, "unable to parse url params")
		return
	}

	var sourceError models.SourceError
	err := json.NewDecoder(r.Body).Decode(&sourceError)
	if err != nil {
		BadRequestError(w, err.Error())
		return
	}

	payload, err := i.DB.Get(params.ExportUUID)
	if err != nil {
		switch err {
		case models.ErrRecordNotFound:
			NotFoundError(w, fmt.Sprintf("record '%s' not found", params.ExportUUID))
			return
		default:
			i.Log.Errorw("error querying for payload entry", "error", err)
			InternalServerError(w, err)
			return
		}
	}

	_, source, err := payload.GetSource(params.ResourceUUID)
	if err != nil {
		i.Log.Errorw("failed to get source: %w", err)
		InternalServerError(w, err.Error())
		return
	}

	if source.Status == models.RSuccess || source.Status == models.RFailed {
		// TODO: revisit this logic and response. Do we want to allow a re-write of an already completed source?
		w.WriteHeader(http.StatusGone)
		Logerr(w.Write([]byte("this resource has already been processed")))
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if err := payload.SetSourceStatus(i.DB, params.ResourceUUID, models.RFailed, &sourceError); err != nil {
		i.Log.Errorw("failed to set source status for failed export", "error", err)
		InternalServerError(w, err)
		return
	}

	if err := payload.SetStatusRunning(i.DB); err != nil {
		i.Log.Errorw("failed to save status update for failed export", "error", err)
		InternalServerError(w, err)
	}

	i.Compressor.ProcessSources(i.DB, params.ExportUUID)
}

// PostUpload receives a POST request from the export source containing
// the exported data. This data is uploaded to S3.
func (i *Internal) PostUpload(w http.ResponseWriter, r *http.Request) {
	i.Log.Info("received payload")
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		InternalServerError(w, "unable to parse url params")
		return
	}

	payload, err := i.DB.Get(params.ExportUUID)
	if err != nil {
		switch err {
		case models.ErrRecordNotFound:
			NotFoundError(w, fmt.Sprintf("record '%s' not found", params.ExportUUID))
			return
		default:
			i.Log.Errorw("error querying for payload entry", "error", err)
			InternalServerError(w, err)
			return
		}
	}

	_, source, err := payload.GetSource(params.ResourceUUID)
	if err != nil {
		i.Log.Errorf("failed to get source: %w", err)
		InternalServerError(w, err.Error())
		return
	}

	if source.Status == models.RSuccess || source.Status == models.RFailed {
		// TODO: revisit this logic and response. Do we want to allow a re-write of an already zipped package?
		w.WriteHeader(http.StatusGone)
		Logerr(w.Write([]byte("this resource has already been processed")))
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if err := i.Compressor.CreateObject(r.Context(), i.DB, r.Body, params.ExportUUID, payload); err != nil {
		Logerr(w.Write([]byte(fmt.Sprintf("payload failed to upload: %v", err))))
	} else {
		Logerr(w.Write([]byte("payload delivered")))
	}

	if err := payload.SetSourceStatus(i.DB, params.ResourceUUID, models.RSuccess, nil); err != nil {
		i.Log.Errorw("failed to set source status for successful export", "error", err)
		InternalServerError(w, err)
	}

	i.Compressor.ProcessSources(i.DB, params.ExportUUID)
}
