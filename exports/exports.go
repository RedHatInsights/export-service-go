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

	"github.com/redhatinsights/export-service-go/errors"
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

	var payload ExportPayload

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		errors.BadRequestError(w, err.Error())
		return
	}

	dbExport, err := APIExportToDBExport(payload)
	if err != nil {
		errors.BadRequestError(w, err.Error())
		return
	}
	payload = DBExportToAPI(*dbExport)

	if len(dbExport.Sources) == 0 {
		errors.BadRequestError(w, "no sources provided")
		return
	}

	dbExport.RequestID = reqID
	dbExport.User = modelUser
	if err := e.DB.Create(dbExport); err != nil {

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
	e.RequestAppResources(r.Context(), r.Header["X-Rh-Identity"][0], *dbExport, e.DB)
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
		errors.BadRequestError(w, err.Error())
		return
	}

	exports, count, err := e.DB.APIList(modelUser, &params, page.Offset, page.Limit, page.SortBy, page.Dir)

	if err != nil {
		errors.InternalServerError(w, err)
		return
	}
	resp, err := middleware.GetPaginatedResponse(r.URL, page, count, exports)
	if err != nil {
		e.Log.Errorw("error while paginating data", "error", err)
		errors.InternalServerError(w, err)
		return
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		e.Log.Errorw("error while encoding", "error", err)
		errors.InternalServerError(w, err.Error())
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
		errors.BadRequestError(w, fmt.Sprintf("'%s' is not ready for download", export.ID))
		return
	}

	out, err := e.StorageHandler.GetObject(r.Context(), export.S3Key)
	if err != nil {
		e.Log.Errorw("failed to get object", "error", err)
		errors.InternalServerError(w, err)
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

	modelUser := mapUsertoModelUser(user)

	if err := e.DB.Delete(exportUUID, modelUser); err != nil {
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

	apiExport := DBExportToAPI(*export)

	if err := json.NewEncoder(w).Encode(&apiExport); err != nil {
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
	modelUser := mapUsertoModelUser(user)

	export, err := e.DB.GetWithUser(exportUUID, modelUser)
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

func DBExportToAPI(payload models.ExportPayload) ExportPayload {
	apiPayload := ExportPayload{
		ID:          payload.ID.String(),
		CreatedAt:   payload.CreatedAt,
		CompletedAt: payload.CompletedAt,
		Expires:     payload.Expires,
		Name:        payload.Name,
		Format:      string(payload.Format),
		Status:      string(payload.Status),
	}
	for _, source := range payload.Sources {
		newSource := Source{
			ID:          source.ID,
			Application: source.Application,
			Status:      string(source.Status),
			Resource:    source.Resource,
			Filters:     source.Filters,
		}

		if source.SourceError != nil {
			newSource.Message = &source.SourceError.Message
			newSource.Code = &source.SourceError.Code
		}

		apiPayload.Sources = append(apiPayload.Sources, newSource)
	}

	return apiPayload
}

func APIExportToDBExport(apiPayload ExportPayload) (*models.ExportPayload, error) {
	payload := models.ExportPayload{
		CreatedAt: apiPayload.CreatedAt,
		Expires:   apiPayload.Expires,
		Name:      apiPayload.Name,
	}

	// use the ID from the request if it's present
	if apiPayload.ID != "" {
		id, err := uuid.Parse(apiPayload.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid export ID: %s", apiPayload.ID)
		}
		payload.ID = id
	} else {
		payload.ID = uuid.New()
	}

	var sources []models.Source
	for _, source := range apiPayload.Sources {
		sources = append(sources, models.Source{
			ID:              uuid.New(),
			ExportPayloadID: payload.ID,
			Application:     source.Application,
			Status:          models.RPending,
			Resource:        source.Resource,
			Filters:         source.Filters,
		})
	}

	payload.Sources = sources

	switch apiPayload.Format {
	case "csv":
		payload.Format = models.CSV
	case "json":
		payload.Format = models.JSON
	default:
		return nil, fmt.Errorf("unknown payload format: %s", apiPayload.Format)
	}

	switch apiPayload.Status {
	case "complete":
		payload.Status = models.Complete
	case "partial":
		payload.Status = models.Partial
	case "failed":
		payload.Status = models.Failed
	default:
		payload.Status = models.Pending // new payloads are pending by default
	}

	return &payload, nil
}
