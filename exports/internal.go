/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

// Internal contains the configuration and
type Internal struct {
	Cfg        *config.ExportConfig
	Compressor *es3.Compressor
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
		errors.InternalServerError(w, "unable to parse url params")
		return
	}

	var sourceError models.SourceError
	err := json.NewDecoder(r.Body).Decode(&sourceError)
	if err != nil {
		errors.BadRequestError(w, err.Error())
		return
	}

	payload := &models.ExportPayload{}
	rows, err := i.DB.Get(params.ExportUUID, payload)
	payload.SetStatusRunning()
	if err != nil {
		i.Log.Errorw("error querying for payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if rows == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", params.ExportUUID))
		return
	}

	source, err := payload.GetSource(params.ResourceUUID)
	if err != nil {
		i.Log.Errorw("failed to get source: %w", err)
		errors.InternalServerError(w, err.Error())
	}

	if source.Status == models.RSuccess || source.Status == models.RFailed {
		// TODO: revisit this logic and response. Do we want to allow a re-write of an already completed source?
		w.WriteHeader(http.StatusGone)
		errors.Logerr(w.Write([]byte("this resource has already been processed")))
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if err := payload.SetSourceStatus(params.ResourceUUID, models.RFailed, &sourceError); err != nil {
		i.Log.Errorw("failed to set source status for failed export", "error", err)
		errors.InternalServerError(w, err)
		return
	}

	if _, err := i.DB.Save(payload); err != nil {
		i.Log.Errorw("failed to save status update for failed export", "error", err)
		errors.InternalServerError(w, err)
	}

	i.processSources(payload)
}

// PostUpload receives a POST request from the export source containing
// the exported data. This data is uploaded to S3.
func (i *Internal) PostUpload(w http.ResponseWriter, r *http.Request) {
	i.Log.Info("received payload")
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		errors.InternalServerError(w, "unable to parse url params")
		return
	}

	payload := &models.ExportPayload{}
	rows, err := i.DB.Get(params.ExportUUID, payload)
	if err != nil {
		i.Log.Errorw("error querying for payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if rows == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", params.ExportUUID))
		return
	}

	if payload.Status == models.Complete || payload.Status == models.Partial {
		// TODO: revisit this logic and response. Do we want to allow a re-write of an already zipped package?
		w.WriteHeader(http.StatusGone)
		errors.Logerr(w.Write([]byte("this export has already been packaged")))
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if err := i.createS3Object(r.Context(), r.Body, params, payload); err != nil {
		errors.Logerr(w.Write([]byte(fmt.Sprintf("payload failed to upload: %v", err))))
	} else {
		errors.Logerr(w.Write([]byte("payload delivered")))
	}
}

func (i *Internal) createS3Object(c context.Context, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) error {
	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, urlparams.ResourceUUID, payload.Format)

	_, uploadErr := i.Compressor.Upload(c, body, &i.Cfg.StorageConfig.Bucket, &filename)
	payload.SetStatusRunning()
	if uploadErr != nil {
		i.Log.Errorf("error during upload: %v", uploadErr)
		statusError := models.SourceError{Message: uploadErr.Error(), Code: 1} // TODO: determine a better approach to assigning an internal status code
		if err := payload.SetSourceStatus(urlparams.ResourceUUID, models.RFailed, &statusError); err != nil {
			i.Log.Errorw("failed to set source status after failed upload", "error", err)
			return uploadErr
		}
		if _, err := i.DB.Save(payload); err != nil {
			i.Log.Errorw("failed to save status update after failed upload", "error", err)
		}
		return uploadErr
	}

	i.Log.Info("successful upload")
	if err := payload.SetSourceStatus(urlparams.ResourceUUID, models.RSuccess, nil); err != nil {
		i.Log.Errorw("failed to set source status after failed upload", "error", err)
		return nil
	}
	if _, err := i.DB.Save(payload); err != nil {
		i.Log.Errorw("failed to save status update after successful upload", "error", err)
		return nil
	}

	i.processSources(payload)

	return nil
}

func (i *Internal) compressPayload(payload *models.ExportPayload) {
	t, filename, s3key, err := i.Compressor.Compress(context.TODO(), payload)
	if err != nil {
		i.Log.Errorw("failed to compress payload", "error", err)
		payload.SetStatusFailed()
	} else {
		i.Log.Infof("done uploading %s", filename)
		ready, err := payload.GetAllSourcesStatus()
		switch ready {
		case models.StatusError:
			i.Log.Errorf("failed to get all source status: %v", err)
		case models.StatusComplete:
			payload.SetStatusComplete(&t, s3key)
		case models.StatusPartial:
			payload.SetStatusPartial(&t, s3key)
		}
	}

	if _, err := i.DB.Save(payload); err != nil {
		i.Log.Errorw("failed updating model status after upload", "error", err)
		return
	}
}

func (i *Internal) processSources(payload *models.ExportPayload) {
	ready, err := payload.GetAllSourcesStatus()
	switch ready {
	case models.StatusError:
		i.Log.Errorf("failed to get all source status: %v", err)
	case models.StatusComplete, models.StatusPartial:
		if payload.Status == models.Running {
			i.Log.Infow("ready for zipping", "export-uuid", payload.ID)
			go i.compressPayload(payload) // start a go-routine to not block
		}
	case models.StatusPending:
		return
	case models.StatusFailed:
		i.Log.Infof("all sources for payload %s reported as failure", payload.ID)
		payload.SetStatusFailed()
		if _, err := i.DB.Save(payload); err != nil {
			i.Log.Errorw("failed updating model status after sources failed", "error", err)
		}
		return
	}
}
