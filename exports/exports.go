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
	"github.com/go-chi/render"
	"github.com/redhatinsights/platform-go-middlewares/request_id"

	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
)

var log = logger.Log

func ExportRouter(r chi.Router) {
	r.Post("/", PostExport)
	r.With(middleware.PaginationCtx).Get("/", ListExports)
	r.Route("/{exportUUID}", func(sub chi.Router) {
		sub.With(middleware.GZIPContentType).Get("/", GetExport) // TODO: will this middleware work correctly?
		sub.Delete("/", DeleteExport)
		sub.Get("/status", GetExportStatus)
	})
}

func PostExport(w http.ResponseWriter, r *http.Request) {
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
	if err := db.DB.Create(&payload).Error; err != nil {
		log.Errorw("error creating payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	if err := json.NewEncoder(w).Encode(&payload); err != nil {
		log.Errorw("error while trying to encode", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}

func buildQuery(q url.Values) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	for k, v := range q {
		if len(v) > 1 {
			return nil, fmt.Errorf("ThIs QuErY iS tOo CoMpLeX")
		}
		result[k] = v[0]
	}

	return result, nil
}

func ListExports(w http.ResponseWriter, r *http.Request) {
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

	exports := []*APIExport{}
	// result := db.DB.Model(
	// 	&models.ExportPayload{}).Where(
	// 	&models.ExportPayload{User: user}).Where(
	// 	query).Find(
	// 	&exports)
	result := db.DB.Model(&models.ExportPayload{}).Where(&models.ExportPayload{User: user}).Find(&exports)
	if result.Error != nil {
		errors.InternalServerError(w, result.Error)
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

func GetExport(w http.ResponseWriter, r *http.Request) {
	// func responsible for downloading from s3
	errors.NotImplementedError(w)
}

func DeleteExport(w http.ResponseWriter, r *http.Request) {
	exportUUID := chi.URLParam(r, "exportUUID")
	if !IsValidUUID(exportUUID) {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid UUID", exportUUID))
		return
	}

	user := middleware.GetUserIdentity(r.Context())

	result := db.DB.Where(&models.ExportPayload{ID: exportUUID, User: user}).Delete(&models.ExportPayload{})
	if result.Error != nil {
		log.Errorw("error deleting payload entry", "error", result.Error)
		errors.InternalServerError(w, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
		return
	}

}

func GetExportStatus(w http.ResponseWriter, r *http.Request) {
	exportUUID := chi.URLParam(r, "exportUUID")
	if !IsValidUUID(exportUUID) {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid UUID", exportUUID))
		return
	}

	user := middleware.GetUserIdentity(r.Context())
	export := APIExport{}

	result := db.DB.Model(&models.ExportPayload{}).Where(&models.ExportPayload{ID: exportUUID, User: user}).Find(&export)
	if result.Error != nil {
		log.Errorw("error querying for payload entry", "error", result.Error)
		errors.InternalServerError(w, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
		return
	}
	if err := json.NewEncoder(w).Encode(&export); err != nil {
		log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}

func NewExportListResponse(exports []*APIExport) []render.Renderer {
	list := []render.Renderer{}
	for _, export := range exports {
		list = append(list, export)
	}
	return list
}
