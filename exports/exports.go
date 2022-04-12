package exports

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi"

	"github.com/maskarb/export-service-go/db"
	"github.com/maskarb/export-service-go/logging"
	"github.com/maskarb/export-service-go/models"
)

var log = logging.Log

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

	// var errors []*IError
	// if err := payload.Validate(); err != nil {
	// 	if e, ok := err.(validation.Errors); ok {
	// 		for k, v := range e {
	// 			var el IError
	// 			el.Field = k
	// 			el.Msg = v.Error()
	// 			errors = append(errors, &el)
	// 		}
	// 	}
	// 	JSONError(w, errors, http.StatusBadRequest)
	// 	return
	// }

	tx := db.DB.Create(&payload)
	if tx.Error != nil {
		log.Error(tx.Error)
	}

}

func ListExports(w http.ResponseWriter, r *http.Request) {}

func GetExport(w http.ResponseWriter, r *http.Request) {}

func DeleteExport(w http.ResponseWriter, r *http.Request) {}

func GetExportStatus(w http.ResponseWriter, r *http.Request) {}

func JSONError(w http.ResponseWriter, err interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(err)
}
