/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"context"
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
	errors.NotImplementedError(w)
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

	if payload.Status == models.Complete {
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
	payload.Status = models.Running
	if uploadErr != nil {
		i.Log.Errorf("error during upload: %w", uploadErr)
		statusMsg := uploadErr.Error()
		if err := payload.SetSourceStatus(urlparams.ResourceUUID, models.RFailed, &statusMsg); err != nil {
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

	ready, uploadErr := payload.GetAllSourcesFinished()
	if uploadErr != nil {
		i.Log.Errorf("failed to get all source status: %w", uploadErr)
	}
	if ready && payload.Status == models.Running {
		i.Log.Infow("ready for zipping", "export-uuid", payload.ID)
		go i.compressPayload(payload) // start a go-routine to not block
	}

	return nil
}

func (i *Internal) compressPayload(payload *models.ExportPayload) {
	t, filename, s3key, err := i.Compressor.Compress(context.TODO(), payload)
	if err != nil {
		i.Log.Errorw("failed to compress payload", "error", err)
		payload.SetStatusFailed()
	} else {
		i.Log.Infof("done uploading %s", filename)
		payload.SetStatusComplete(&t, s3key)
	}

	if _, err := i.DB.Save(payload); err != nil {
		i.Log.Errorw("failed updating model status after upload", "error", err)
		return
	}
}
