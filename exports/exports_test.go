package exports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

func GenerateExportRequestBody(name, format, sources string) (exportRequest []byte) {
	return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "sources": [%s]}`, name, format, sources))
}

func CreateExportRequest(name, format, sources string) *http.Request {
	exportRequest := GenerateExportRequestBody(name, format, sources)
	request, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequest))
	Expect(err).To(BeNil())
	request.Header.Set("Content-Type", "application/json")
	return request
}

var _ = Context("Set up export handler", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	var exportHandler *exports.Export
	var router *chi.Mux

	var mu sync.Mutex // required to prevent data race when tests are run in parallel
	ResourceRequest := false
	madeResourceRequest := &ResourceRequest
	var mockRequestApplicationResources = func(mr *bool) exports.RequestApplicationResources {
		return func(ctx context.Context, identity string, payload models.ExportPayload) error {
			mu.Lock()
			defer mu.Unlock()
			*mr = true
			fmt.Println("KAFKA MESSAGE SENT: ", *mr)
			return nil
		}
	}
	requestAppResources := mockRequestApplicationResources(madeResourceRequest)

	BeforeEach(func() {
		exportHandler = &exports.Export{
			Bucket:              cfg.StorageConfig.Bucket,
			Client:              es3.Client,
			DB:                  &models.ExportDB{DB: testGormDB},
			RequestAppResources: requestAppResources,
			Log:                 logger.Log,
		}

		router = chi.NewRouter()
		router.Use(
			emiddleware.InjectDebugUserIdentity,
			identity.EnforceIdentity,
			emiddleware.EnforceUserIdentity,
		)

		router.Route("/api/export/v1", func(sub chi.Router) {
			sub.Post("/exports", exportHandler.PostExport)
			sub.With(emiddleware.PaginationCtx).Get("/exports", exportHandler.ListExports)
			sub.Get("/exports/{exportUUID}/status", exportHandler.GetExportStatus)
			sub.Delete("/exports/{exportUUID}", exportHandler.DeleteExport)
			sub.Get("/exports/{exportUUID}", exportHandler.GetExport)
		})
	})

	Describe("The public API", func() {

		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			testGormDB.Exec("DELETE FROM export_payloads")
			mu.Lock()
			defer mu.Unlock()
			*madeResourceRequest = false
		})

		DescribeTable("can create a new export request", func(name, format, sources, expectedBody string, expectedStatus int) {
			rr := httptest.NewRecorder()

			req := CreateExportRequest(name, format, sources)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(expectedStatus))
			Expect(rr.Body.String()).To(ContainSubstring(expectedBody))
		},
			Entry("with valid request", "Test Export Request", "json", `{"application":"exampleApp", "resource":"exampleResource", "expires":"2023-01-01T00:00:00Z"}`, "", http.StatusAccepted),
			Entry("with no expiration", "Test Export Request", "json", `{"application":"exampleApp", "resource":"exampleResource"}`, "", http.StatusAccepted),
			Entry("with an invalid format", "Test Export Request", "abcde", `{"application":"exampleApp", "resource":"exampleResource", "expires":"2023-01-01T00:00:00Z"}`, "unknown payload format", http.StatusBadRequest),
			Entry("With no sources", "Test Export Request", "json", "", "no sources provided", http.StatusBadRequest),
		)

		It("can list all export requests", func() {
			rr := httptest.NewRecorder()

			// Generate 3 export requests
			for i := 1; i <= 3; i++ {
				req := CreateExportRequest(
					fmt.Sprintf("Test Export Request %d", i),
					"json",
					`{"application":"exampleApp", "resource":"exampleResource"}`,
				)

				router.ServeHTTP(rr, req)
				Expect(rr.Code).To(Equal(http.StatusAccepted))
			}

			req, err := http.NewRequest("GET", "/api/export/v1/exports", nil)
			req.Header.Set("Content-Type", "application/json")
			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
		})

		It("can check the status of an export request", func() {
			rr := httptest.NewRecorder()

			req := CreateExportRequest(
				"Test Export Request",
				"json",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// Grab the 'id' from the response
			var exportResponse map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
			Expect(err).ShouldNot(HaveOccurred())
			exportUUID := exportResponse["id"].(string)

			// Check the status of the export request
			req, err = http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s/status", exportUUID), nil)
			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(rr.Body.String()).To(ContainSubstring(`"status":"pending"`))
		})

		// TODO:
		It("sends a request message to the export sources", func() {
			rr := httptest.NewRecorder()

			mu.Lock()
			defer mu.Unlock()

			req := CreateExportRequest(
				"Test Export Request",
				"json",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)

			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(madeResourceRequest).To(BeTrue())

		})
		// It("can get a completed export request by ID and download it")

		It("can delete a specific export request by ID", func() {
			rr := httptest.NewRecorder()

			req := CreateExportRequest(
				"Test Export Request",
				"json",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// Grab the 'id' from the export request
			var exportResponse map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
			Expect(err).ShouldNot(HaveOccurred())
			exportUUID := exportResponse["id"].(string)

			fmt.Println("EXPORT UUID: ", exportUUID)

			// Delete the export request
			req, err = http.NewRequest("DELETE", fmt.Sprintf("/api/export/v1/exports/%s", exportUUID), nil)
			req.Header.Set("Content-Type", "application/json")
			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)
			fmt.Print("Response Body: ", rr.Body.String())
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			// Check that the export was deleted
			rr = httptest.NewRecorder()
			req, err = http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s", exportUUID), nil)
			req.Header.Set("Content-Type", "application/json")
			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusNotFound))
			Expect(rr.Body.String()).To(ContainSubstring("not found"))
		})
	})
})
