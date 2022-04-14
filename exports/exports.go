package exports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
)

var log = logger.Log

type APIExport struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created"`
	CompletedAt *time.Time `json:"completed,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
}

func ExportRouter(r chi.Router) {
	r.Post("/", PostExport)
	r.Get("/", ListExports)
	r.Route("/{exportUUID}", func(sub chi.Router) {
		sub.Get("/", GetExport)
		sub.Delete("/", DeleteExport)
		sub.Get("/status", GetExportStatus)
	})
}

func PostExport(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserIdentity(r.Context())

	var payload models.ExportPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		errors.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.User = user
	tx := db.DB.Create(&payload)
	if tx.Error != nil {
		log.Error(tx.Error)
		errors.JSONError(w, tx.Error, http.StatusBadRequest)
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&payload); err != nil {
		log.Error("Error while trying to encode")
		errors.JSONError(w, err.Error(), http.StatusBadRequest)
	}
}

func ListExports(w http.ResponseWriter, r *http.Request) {
	exports := []*APIExport{}
	result := db.DB.Model(&models.ExportPayload{}).Find(&exports)
	if result.Error != nil {
		errors.JSONError(w, result.Error, http.StatusBadRequest)
		return
	}
	if err := render.RenderList(w, r, NewExportListResponse(exports)); err != nil {
		errors.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func GetExport(w http.ResponseWriter, r *http.Request) {
	// func responsible for downloading from s3
}

func DeleteExport(w http.ResponseWriter, r *http.Request) {
	if exportUUID := chi.URLParam(r, "exportUUID"); exportUUID != "" {
		result := db.DB.Delete(&models.ExportPayload{}, "id = ?", exportUUID)
		if result.Error != nil {
			log.Error(result.Error)
			errors.JSONError(w, result.Error, http.StatusBadRequest)
			return
		}
		if result.RowsAffected == 0 {
			errors.JSONError(w, fmt.Sprintf("record %s not found", exportUUID), http.StatusNotFound)
			return
		}
	}
}

func GetExportStatus(w http.ResponseWriter, r *http.Request) {
	export := APIExport{}
	if exportUUID := chi.URLParam(r, "exportUUID"); exportUUID != "" {
		result := db.DB.Model(&models.ExportPayload{}).Find(&export, "id = ?", exportUUID)
		if result.Error != nil {
			log.Error(result.Error)
			errors.JSONError(w, result.Error, http.StatusBadRequest)
			return
		}
		if result.RowsAffected == 0 {
			errors.JSONError(w, fmt.Sprintf("record %s not found", exportUUID), http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(&export); err != nil {
			log.Error("Error while trying to encode")
			errors.JSONError(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func (e *APIExport) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func NewExportListResponse(exports []*APIExport) []render.Renderer {
	list := []render.Renderer{}
	for _, export := range exports {
		list = append(list, export)
	}
	return list
}
