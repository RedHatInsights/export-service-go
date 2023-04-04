package exports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

var _ = Context("Set up internal handler", func() {
	cfg := config.Get()
	log := logger.Get()

	var internalHandler *exports.Internal
	var router *chi.Mux

	BeforeEach(func() {

		internalHandler = &exports.Internal{
			Cfg:        cfg,
			Compressor: &es3.MockStorageHandler{},
			DB:         &models.ExportDB{DB: testGormDB},
			Log:        log,
		}

		mockKafkaCall := func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
		}

		exportHandler := &exports.Export{
			Bucket:              "cfg.StorageConfig.Bucket",
			StorageHandler:      &es3.MockStorageHandler{},
			DB:                  &models.ExportDB{DB: testGormDB},
			RequestAppResources: mockKafkaCall,
			Log:                 log,
		}

		router = chi.NewRouter()
		router.Use(
			identity.EnforceIdentity,
			emiddleware.EnforceUserIdentity,
		)

		router.Route("/app/export/v1", func(sub chi.Router) {
			sub.With(emiddleware.URLParamsCtx).Post("/upload/{exportUUID}/{application}/{resourceUUID}", internalHandler.PostUpload)
			sub.With(emiddleware.URLParamsCtx).Post("/error/{exportUUID}/{application}/{resourceUUID}", internalHandler.PostError)
		})

		router.Route("/api/export/v1", func(sub chi.Router) {
			sub.Post("/exports", exportHandler.PostExport)
			sub.With(emiddleware.PaginationCtx).Get("/exports", exportHandler.ListExports)
			sub.Get("/exports/{exportUUID}/status", exportHandler.GetExportStatus)
			sub.Delete("/exports/{exportUUID}", exportHandler.DeleteExport)
			sub.Get("/exports/{exportUUID}", exportHandler.GetExport)
		})
	})

	Describe("The internal API", func() {
		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			testGormDB.Exec("DELETE FROM export_payloads")
		})

		It("allows the user to upload a payload", func() {
			rr := httptest.NewRecorder()

			req := createExportRequest("testRequest", "json", "", `{"application":"exampleApp", "resource":"exampleResource"}`)
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// grab the 'id' from the response
			// Example: {"id":"288b57e9-e776-46e3-827d-9ed94fd36a6b","created":"2022-12-13T14:37:14.573655756-05:00","name":"testRequest","format":"json","status":"pending","sources":[{"id":"1663cd53-4b72-4c9d-98a7-8433595723df","application":"exampleApp","status":"pending","resource":"exampleResource","filters":null}]}
			var exportResponse map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
			Expect(err).ShouldNot(HaveOccurred())
			exportUUID := exportResponse["id"].(string)
			sources := exportResponse["sources"].([]interface{})
			source := sources[0].(map[string]interface{})
			resourceUUID := source["id"].(string)

			// upload a payload with some dummy data
			rr = httptest.NewRecorder()
			dummyBody := `{"data": "dummy data"}`
			req = httptest.NewRequest("POST", fmt.Sprintf("/app/export/v1/upload/%s/exampleApp/%s", exportUUID, resourceUUID), bytes.NewBuffer([]byte(dummyBody)))
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// check that the status of the export is now 'complete'
			rr = httptest.NewRecorder()
			req = httptest.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s/status", exportUUID), nil)
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusOK))

			var exportResponse2 map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &exportResponse2)
			Expect(err).ShouldNot(HaveOccurred())

			exportStatus := exportResponse2["status"].(string)
			Expect(exportStatus).To(Equal("complete"))
		})

		It("allows the user to return an error when the export request is invalid", func() {
			rr := httptest.NewRecorder()

			req := createExportRequest("testRequest", "json", "2023-01-01T00:00:00Z", `{"application":"exampleApp", "resource":"exampleResource"}`)
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// grab the 'id' from the response
			// Example: {"id":"288b57e9-e776-46e3-827d-9ed94fd36a6b","created":"2022-12-13T14:37:14.573655756-05:00","name":"testRequest","format":"json","status":"pending","sources":[{"id":"1663cd53-4b72-4c9d-98a7-8433595723df","application":"exampleApp","status":"pending","resource":"exampleResource","filters":null}]}
			var exportResponse map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
			Expect(err).ShouldNot(HaveOccurred())
			exportUUID := exportResponse["id"].(string)
			sources := exportResponse["sources"].([]interface{})
			source := sources[0].(map[string]interface{})
			resourceUUID := source["id"].(string)
			fmt.Println(resourceUUID)

			// upload a payload with some dummy data
			rr = httptest.NewRecorder()
			dummyBody := `{"data": "dummy data"}`
			req = httptest.NewRequest("POST", fmt.Sprintf("/app/export/v1/error/%s/exampleApp/%s", exportUUID, resourceUUID), bytes.NewBuffer([]byte(dummyBody)))
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// check that the status of the export is now 'complete'
			rr = httptest.NewRecorder()
			req = httptest.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s/status", exportUUID), nil)
			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusOK))
			var exportResponse2 map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &exportResponse2)
			Expect(err).ShouldNot(HaveOccurred())
			exportStatus := exportResponse2["status"].(string)
			Expect(exportStatus).To(Equal("failed"))
		})
	})
})
