/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

// Export holds any dependencies necessary for the external api endpoints
type Export struct {
	Bucket              string
	Client              *s3.Client
	DB                  models.DBInterface
	Log                 *zap.SugaredLogger
	RequestAppResources RequestApplicationResources
}

// ExportRouter is a router for all of the external routes for the /exports endpoint.
func (e *Export) ExportRouter(r chi.Router) {
	r.Post("/", e.PostExport)
	r.With(middleware.PaginationCtx).Get("/", e.ListExports)
	r.Route("/{exportUUID}", func(sub chi.Router) {
		sub.With(middleware.GZIPContentType).Get("/", e.GetExport)
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

	sources, err := payload.GetSources()
	if err != nil {
		errors.BadRequestError(w, err.Error())
		return
	}
	if len(sources) == 0 {
		errors.BadRequestError(w, "no sources provided")
		return
	}

	payload.RequestID = reqID
	payload.User = user
	if err := e.DB.Create(&payload); err != nil {
		e.Log.Errorw("error creating payload entry", "error", err)
		errors.InternalServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(&payload); err != nil {
		e.Log.Errorw("error while trying to encode", "error", err)
		errors.InternalServerError(w, err.Error())
	}

	// send the payload to the producer with a goroutine so
	// that we do not block the response
	go e.RequestAppResources(r.Context(), r.Header["X-Rh-Identity"][0], payload)
}

// func buildQuery(q url.Values) (map[string]interface{}, error) {
// 	result := map[string]interface{}{}

// 	for k, v := range q {
// 		if len(v) > 1 {
// 			return nil, fmt.Errorf("query param `%s` has too many search values", k)
// 		}
// 		result[k] = v[0]
// 	}

// 	return result, nil
// }

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
		e.Log.Errorw("error while paginating data", "error", err)
		errors.InternalServerError(w, err)
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		e.Log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}

// GetExport handles GET requests to the /exports/{exportUUID} endpoint.
// This function is responsible for returning the S3 object.
func (e *Export) GetExport(w http.ResponseWriter, r *http.Request) {
	export := e.getExportWithUser(w, r)
	if export == nil {
		return
	}
	if export.Status != models.Complete && export.Status != models.Partial {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not ready for download", export.ID))
		return
	}

	input := s3.GetObjectInput{Bucket: &e.Bucket, Key: &export.S3Key}

	out, err := es3.GetObject(r.Context(), e.Client, &input)
	if err != nil {
		e.Log.Errorw("failed to get object", "error", err)
		errors.InternalServerError(w, err)
		return
	}

	baseName := filepath.Base(export.S3Key)
	w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", baseName))
	w.WriteHeader(http.StatusOK)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out.Body); err != nil {
		e.Log.Errorf("failed to read body: %w", err)
	}
	errors.Logerr(w.Write(buf.Bytes()))
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

	if err := e.DB.Delete(exportUUID, user); err != nil {
		switch err {
		case models.ErrRecordNotFound:
			errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return
		default:
			e.Log.Errorw("error deleting payload entry", "error", err)
			errors.InternalServerError(w, err)
			return
		}
	}
}

// GetExportStatus handles GET requests to the /exports/{exportUUID}/status endpoint.
func (e *Export) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	export := e.getExportWithUser(w, r)
	if export == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(&export); err != nil {
		e.Log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
	}
}

func (e *Export) getExportWithUser(w http.ResponseWriter, r *http.Request) *models.ExportPayload {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return nil
	}

	user := middleware.GetUserIdentity(r.Context())

	export, err := e.DB.GetWithUser(exportUUID, user)
	if err != nil {
		switch err {
		case models.ErrRecordNotFound:
			e.Log.Infof("record '%s' not found", exportUUID)
			errors.NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return nil
		default:
			e.Log.Errorw("error querying for payload entry", "error", err)
			errors.InternalServerError(w, err)
			return nil
		}
	}

	return export
}
