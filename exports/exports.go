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

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

// Export holds any dependencies necessary for the external api endpoints
type Export struct {
	Bucket              string
	StorageHandler      es3.StorageHandler
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

func mapUsertoModelUser(user middleware.User) models.User {
	modelUser := models.User{
		AccountID:      user.AccountID,
		OrganizationID: user.OrganizationID,
		Username:       user.Username,
	}
	return modelUser
}

// PostExport handles POST requests to the /exports endpoint.
func (e *Export) PostExport(w http.ResponseWriter, r *http.Request) {
	reqID := request_id.GetReqID(r.Context())
	user := middleware.GetUserIdentity(r.Context())

	modelUser := mapUsertoModelUser(user)

	var payload models.ExportPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		BadRequestError(w, err.Error())
		return
	}

	sources, err := payload.GetSources()
	if err != nil {
		BadRequestError(w, err.Error())
		return
	}
	if len(sources) == 0 {
		BadRequestError(w, "no sources provided")
		return
	}

	payload.RequestID = reqID
	payload.User = modelUser
	if err := e.DB.Create(&payload); err != nil {
		e.Log.Errorw("error creating payload entry", "error", err)
		InternalServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(&payload); err != nil {
		e.Log.Errorw("error while trying to encode", "error", err)
		InternalServerError(w, err.Error())
	}

	// send the payload to the producer with a goroutine so
	// that we do not block the response
	e.RequestAppResources(r.Context(), r.Header["X-Rh-Identity"][0], payload)
}

// ListExports handle GET requests to the /exports endpoint.
func (e *Export) ListExports(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserIdentity(r.Context())
	page := middleware.GetPagination(r.Context())

	modelUser := mapUsertoModelUser(user)

	q := r.URL.Query()

	params, err := initQuery(q)
	if err != nil {
		e.Log.Errorw("error while parsing params", "error", err)
		BadRequestError(w, err.Error())
		return
	}

	exports, count, err := e.DB.APIList(modelUser, &params, page.Offset, page.Limit, page.SortBy, page.Dir)
	if err != nil {
		InternalServerError(w, err)
		return
	}
	resp, err := middleware.GetPaginatedResponse(r.URL, page, count, exports)
	if err != nil {
		e.Log.Errorw("error while paginating data", "error", err)
		InternalServerError(w, err)
		return
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		e.Log.Errorw("error while encoding", "error", err)
		InternalServerError(w, err.Error())
		return
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
		BadRequestError(w, fmt.Sprintf("'%s' is not ready for download", export.ID))
		return
	}

	out, err := e.StorageHandler.GetObject(r.Context(), export.S3Key)
	if err != nil {
		e.Log.Errorw("failed to get object", "error", err)
		InternalServerError(w, err)
		return
	}

	baseName := filepath.Base(export.S3Key)
	w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", baseName))
	w.WriteHeader(http.StatusOK)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out); err != nil {
		e.Log.Errorf("failed to read body: %w", err)
	}
	err = out.Close()
	if err != nil {
		e.Log.Errorf("failed to close body: %w", err)
	}
	Logerr(w.Write(buf.Bytes()))
}

// DeleteExport handles DELETE requests to the /exports/{exportUUID} endpoint.
func (e *Export) DeleteExport(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return
	}

	user := middleware.GetUserIdentity(r.Context())

	modelUser := mapUsertoModelUser(user)

	if err := e.DB.Delete(exportUUID, modelUser); err != nil {
		switch err {
		case models.ErrRecordNotFound:
			NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return
		default:
			e.Log.Errorw("error deleting payload entry", "error", err)
			InternalServerError(w, err)
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
		InternalServerError(w, err.Error())
	}
}

func (e *Export) getExportWithUser(w http.ResponseWriter, r *http.Request) *models.ExportPayload {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return nil
	}

	user := middleware.GetUserIdentity(r.Context())
	modelUser := mapUsertoModelUser(user)

	export, err := e.DB.GetWithUser(exportUUID, modelUser)
	if err != nil {
		switch err {
		case models.ErrRecordNotFound:
			e.Log.Infof("record '%s' not found", exportUUID)
			NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return nil
		default:
			e.Log.Errorw("error querying for payload entry", "error", err)
			InternalServerError(w, err)
			return nil
		}
	}

	return export
}
