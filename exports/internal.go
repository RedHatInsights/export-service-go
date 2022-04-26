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

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

var cfg = config.ExportCfg
var client = es3.Client

// InternalRouter is a router for all of the internal routes which require exportuuid,
// application name, and resourceuuid.
func InternalRouter(r chi.Router) {
	r.Route("/{exportUUID}/{application}/{resourceUUID}", func(sub chi.Router) {
		sub.Use(middleware.URLParamsCtx)
		sub.Post("/upload", PostUpload)
		sub.Post("/error", PostError)
	})
}

// PostError receives a POST request from the export source which contains the
// errors associated with creating the export for the given resource.
func PostError(w http.ResponseWriter, r *http.Request) {
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		errors.InternalServerError(w, "unable to parse url params")
		return
	}
	errors.NotImplementedError(w)
}

// PostUpload receives a POST request from the export source containing
// the exported data. This data is uploaded to S3.
func PostUpload(w http.ResponseWriter, r *http.Request) {
	log.Info("received payload")
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		errors.InternalServerError(w, "unable to parse url params")
		return
	}

	payload := &models.ExportPayload{}
	result := db.DB.Model(&models.ExportPayload{}).Where(&models.ExportPayload{ID: params.ExportUUID}).Find(&payload)
	if result.Error != nil {
		log.Errorw("error querying for payload entry", "error", result.Error)
		errors.InternalServerError(w, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", params.ExportUUID))
		return
	}
	w.WriteHeader(http.StatusAccepted)

	if err := createS3Object(r.Context(), r.Body, params, payload); err != nil {
		w.Write([]byte(fmt.Sprintf("payload failed to upload: %v", err)))
	} else {
		w.Write([]byte("payload delivered"))
	}
}

func createS3Object(c context.Context, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) error {

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, urlparams.ResourceUUID, payload.Format)

	input := &s3.PutObjectInput{
		Bucket: &cfg.StorageConfig.Bucket,
		Key:    &filename,
		Body:   body,
	}
	_, uploadErr := uploader.Upload(c, input)
	if uploadErr != nil {
		log.Errorf("error during upload: %v", uploadErr)
		statusMsg := uploadErr.Error()
		if err := payload.SetSourceStatus(urlparams.ResourceUUID, models.RFailed, &statusMsg); err != nil {
			log.Errorw("failed to set source status after failed upload", "error", err)
			return uploadErr
		}
		if err := db.DB.Save(payload).Error; err != nil {
			log.Errorw("failed to save status update after failed upload", "error", err)
		}
		return uploadErr
	}

	log.Info("successful upload")
	if err := payload.SetSourceStatus(urlparams.ResourceUUID, models.RSuccess, nil); err != nil {
		log.Errorw("failed to set source status after failed upload", "error", err)
		return nil
	}
	if err := db.DB.Save(payload).Error; err != nil {
		log.Errorw("failed to save status update after successful upload", "error", err)
		return nil
	}

	ready, uploadErr := payload.GetAllSourcesSuccess()
	if uploadErr != nil {
		log.Errorf("failed to get all source status: %v", uploadErr)
	}
	if ready {
		log.Info("ready for zipping")
	}
	return nil
}
