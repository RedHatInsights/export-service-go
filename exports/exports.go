package exports

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"

	"github.com/maskarb/export-service-go/db"
	"github.com/maskarb/export-service-go/logging"
	"github.com/maskarb/export-service-go/models"
)

var log = logging.Log

type APIExport struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created"`
	CompletedAt *time.Time `json:"completed,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
}

type IError struct {
	Field string
	Msg   string
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
	var payload models.ExportPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	tx := db.DB.Create(&payload)
	if tx.Error != nil {
		log.Error(tx.Error)
	}
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&payload); err != nil {
		log.Error("Error while trying to encode")
	}

}

func ListExports(w http.ResponseWriter, r *http.Request) {
	exports := []*APIExport{}
	result := db.DB.Model(&models.ExportPayload{}).Find(&exports)
	if result.Error != nil {
		JSONError(w, result.Error, http.StatusBadRequest)
		return
	}
	if err := render.RenderList(w, r, NewExportListResponse(exports)); err != nil {
		JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func GetExport(w http.ResponseWriter, r *http.Request) {}

func DeleteExport(w http.ResponseWriter, r *http.Request) {}

func GetExportStatus(w http.ResponseWriter, r *http.Request) {}

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

func JSONError(w http.ResponseWriter, err interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(err)
}
