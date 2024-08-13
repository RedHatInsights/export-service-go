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

	"github.com/redhatinsights/export-service-go/config"
	export_logger "github.com/redhatinsights/export-service-go/logger"
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

	logger := e.Log.With(export_logger.RequestIDField(reqID), export_logger.OrgIDField(user.OrganizationID))

	var apiExport ExportPayload
	err := json.NewDecoder(r.Body).Decode(&apiExport)
	if err != nil {
		logger.Errorw("error while parsing params", "error", err)
		BadRequestError(w, err.Error())
		return
	}

	if len(apiExport.Sources) == 0 {
		logger.Errorw("no sources provided", "error", err)
		BadRequestError(w, "no sources provided")
		return
	}

	cfg := config.Get()

	if err = verifyExportableApplication(cfg.ExportableApplications, apiExport.Sources); err != nil {
		logger.Errorw("Payload does not match Configured Exports", "error", err)
		StatusNotAcceptableError(w, "Payload does not match Configured Exports")
		return
	}

	dbExport, err := APIExportToDBExport(apiExport)
	if err != nil {
		logger.Errorw("unable to convert api export into db export", "error", err)
		BadRequestError(w, err.Error())
		return
	}

	dbExport.RequestID = reqID
	dbExport.User = modelUser

	dbExport, err = e.DB.Create(dbExport)
	if err != nil {
		logger.Errorw("error creating payload entry", "error", err)
		InternalServerError(w, err)
		return
	}

	logger = logger.With(export_logger.ExportIDField(dbExport.ID.String()))

	w.WriteHeader(http.StatusAccepted)

	apiExport = DBExportToAPI(*dbExport)
	if err := json.NewEncoder(w).Encode(&apiExport); err != nil {
		logger.Errorw("error while trying to encode", "error", err)
		InternalServerError(w, err.Error())
	}

	// send the payload to the producer with a goroutine so
	// that we do not block the response
	e.RequestAppResources(r.Context(), logger, r.Header["X-Rh-Identity"][0], *dbExport)
}

// verifyEdportableApplications verifies if an application or resource is in the map
func verifyExportableApplication(exportableApplications map[string]map[string]bool, payloadSources []Source) error {
	for _, source := range payloadSources {

		_, ok := exportableApplications[source.Application]
		if !ok {
			return fmt.Errorf("invalid application")
		}
		_, ok = exportableApplications[source.Application][source.Resource]
		if !ok {
			return fmt.Errorf("invalid resource")
		}
	}
	return nil
}

// ListExports handle GET requests to the /exports endpoint.
func (e *Export) ListExports(w http.ResponseWriter, r *http.Request) {
	reqID := request_id.GetReqID(r.Context())
	user := middleware.GetUserIdentity(r.Context())
	page := middleware.GetPagination(r.Context())

	modelUser := mapUsertoModelUser(user)

	logger := e.Log.With(export_logger.RequestIDField(reqID), export_logger.OrgIDField(user.OrganizationID))

	q := r.URL.Query()

	params, err := initQuery(q)
	if err != nil {
		logger.Errorw("error while parsing params", "error", err)
		BadRequestError(w, err.Error())
		return
	}

	exports, count, err := e.DB.APIList(modelUser, &params, page.Offset, page.Limit, page.SortBy, page.Dir)

	if err != nil {
		logger.Errorw("error while retrieving list from database", "error", err)
		InternalServerError(w, err)
		return
	}
	resp, err := middleware.GetPaginatedResponse(r.URL, page, count, exports)
	if err != nil {
		logger.Errorw("error while paginating data", "error", err)
		InternalServerError(w, err)
		return
	}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		logger.Errorw("error while encoding", "error", err)
		InternalServerError(w, err.Error())
		return
	}
}

// GetExport handles GET requests to the /exports/{exportUUID} endpoint.
// This function is responsible for returning the S3 object.
func (e *Export) GetExport(w http.ResponseWriter, r *http.Request) {
	reqID := request_id.GetReqID(r.Context())
	user := middleware.GetUserIdentity(r.Context())

	logger := e.Log.With(export_logger.RequestIDField(reqID), export_logger.OrgIDField(user.OrganizationID))

	export := e.getExportWithUser(w, r, logger)
	if export == nil {
		return
	}

	if export.Status != models.Complete && export.Status != models.Partial {
		logger.Infof("'%s' not ready for download", export.ID)
		BadRequestError(w, fmt.Sprintf("'%s' is not ready for download", export.ID))
		return
	}

	out, err := e.StorageHandler.GetObject(r.Context(), export.S3Key)
	if err != nil {
		logger.Errorw("failed to get object", "error", err)
		InternalServerError(w, err)
		return
	}

	baseName := filepath.Base(export.S3Key)
	w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", baseName))
	w.WriteHeader(http.StatusOK)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out); err != nil {
		logger.Errorf("failed to read body: %w", err)
	}
	err = out.Close()
	if err != nil {
		logger.Errorf("failed to close body: %w", err)
	}
	Logerr(w.Write(buf.Bytes()))
}

// DeleteExport handles DELETE requests to the /exports/{exportUUID} endpoint.
func (e *Export) DeleteExport(w http.ResponseWriter, r *http.Request) {

	user := middleware.GetUserIdentity(r.Context())
	reqID := request_id.GetReqID(r.Context())

	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return
	}

	logger := e.Log.With(export_logger.RequestIDField(reqID), export_logger.ExportIDField(uid), export_logger.OrgIDField(user.OrganizationID))

	modelUser := mapUsertoModelUser(user)

	if err := e.DB.Delete(exportUUID, modelUser); err != nil {
		switch err {
		case models.ErrRecordNotFound:
			NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return
		default:
			logger.Errorw("error deleting payload entry", "error", err)
			InternalServerError(w, err)
			return
		}
	}
}

// GetExportStatus handles GET requests to the /exports/{exportUUID}/status endpoint.
func (e *Export) GetExportStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserIdentity(r.Context())
	reqID := request_id.GetReqID(r.Context())

	logger := e.Log.With(export_logger.RequestIDField(reqID), export_logger.OrgIDField(user.OrganizationID))

	export := e.getExportWithUser(w, r, logger)
	if export == nil {
		return
	}

	apiExport := DBExportToAPI(*export)

	if err := json.NewEncoder(w).Encode(&apiExport); err != nil {
		logger.Errorw("error while encoding", "error", err)
		InternalServerError(w, err.Error())
	}
}

func (e *Export) getExportWithUser(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) *models.ExportPayload {
	uid := chi.URLParam(r, "exportUUID")
	exportUUID, err := uuid.Parse(uid)
	if err != nil {
		BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
		return nil
	}

	logger = logger.With(export_logger.ExportIDField(uid))

	user := middleware.GetUserIdentity(r.Context())
	modelUser := mapUsertoModelUser(user)

	export, err := e.DB.GetWithUser(exportUUID, modelUser)
	if err != nil {
		switch err {
		case models.ErrRecordNotFound:
			logger.Infof("record '%s' not found", exportUUID)
			NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
			return nil
		default:
			logger.Errorw("error querying for payload entry", "error", err)
			InternalServerError(w, err)
			return nil
		}
	}

	return export
}

func DBExportToAPI(payload models.ExportPayload) ExportPayload {
	// UTC required to format as ISO 8601
	payload.CreatedAt = payload.CreatedAt.UTC()
	if payload.CompletedAt != nil {
		*payload.CompletedAt = payload.CompletedAt.UTC()
	}
	if payload.Expires != nil {
		*payload.Expires = payload.Expires.UTC()
	}

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
			newSource.Message = source.SourceError.Message
			newSource.Code = source.SourceError.Code
		}

		apiPayload.Sources = append(apiPayload.Sources, newSource)
	}

	return apiPayload
}

func APIExportToDBExport(apiPayload ExportPayload) (*models.ExportPayload, error) {
	payload := models.ExportPayload{
		Name: apiPayload.Name,
	}

	var sources []models.Source
	for _, source := range apiPayload.Sources {

		if source.Filters != nil {
			var dst any
			// Verify the incoming filters are valid json
			err := json.Unmarshal(source.Filters, &dst)
			if err != nil {
				return nil, fmt.Errorf("invalid json format of filters")
			}
		}

		sources = append(sources, models.Source{
			Application: source.Application,
			Status:      models.RPending,
			Resource:    source.Resource,
			Filters:     source.Filters,
		})
	}

	payload.Sources = sources

	if apiPayload.Expires != nil {
		payload.Expires = apiPayload.Expires
	}

	switch apiPayload.Format {
	case "csv":
		payload.Format = models.CSV
	case "json":
		payload.Format = models.JSON
	default:
		return nil, fmt.Errorf("invalid or missing payload format")
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
