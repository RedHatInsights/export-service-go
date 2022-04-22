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
	params := middleware.GetURLParams(r.Context())
	if params == nil {
		errors.InternalServerError(w, "unable to parse url params")
		return
	}
	if err := createS3Object(r.Context(), r.Body, params); err != nil {
		log.Errorw("error creating s3 object", "error", err)
		errors.InternalServerError(w, err)
		return
	}

}

func createS3Object(c context.Context, body io.Reader, params *models.URLParams) error {

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024 // 10 MiB
	})

	filename := fmt.Sprintf("%s/%s/%s.%s", "10001", params.ExportUUID, params.ResourceUUID, "csv")

	input := &s3.PutObjectInput{
		Bucket: &cfg.StorageConfig.Bucket,
		Key:    &filename,
		Body:   body,
	}
	_, err := uploader.Upload(c, input)
	if err != nil {
		return fmt.Errorf("error uploading file: %v", err)
	}

	// update resource status to success

	return nil
}
