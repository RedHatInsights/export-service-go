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
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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

	createS3Object(r.Context(), r.Body, params, payload)

	w.Write([]byte("payload delivered"))

}

func worker(id int, jobs <-chan int, results chan<- int) {
	for j := range jobs {
		fmt.Println("worker", id, "started  job", j)
		time.Sleep(time.Second)
		fmt.Println("worker", id, "finished job", j)
		results <- j * 2
	}
}

func createS3Object(c context.Context, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) {

	// source := export.Sources[0]

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024 // 10 MiB
	})

	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, urlparams.ResourceUUID, payload.Format)
	time.Sleep(10 * time.Second)

	input := &s3.PutObjectInput{
		Bucket: &cfg.StorageConfig.Bucket,
		Key:    &filename,
		Body:   body,
	}
	_, err := uploader.Upload(c, input)
	if err != nil {
		log.Errorf("error during upload: %v", err)
		// db.DB.Model(&models.ExportSource{}).Where("id = ?", export.SourceID.String()).Update("status", models.RFailure)
		return
	}

	log.Info("successful upload")
	// db.DB.Model(&models.ExportSource{}).Where("id = ?", export.SourceID).Update("status", models.RSuccess)

}

type Helper struct {
	PayloadID uuid.UUID
	SourceID  uuid.UUID
	OrgID     string
	Format    string
}
