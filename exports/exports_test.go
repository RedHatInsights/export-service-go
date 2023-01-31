package exports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhatinsights/platform-go-middlewares/identity"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

func generateExportRequestBody(name, format, sources string) (exportRequest []byte) {
	return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "sources": [%s]}`, name, format, sources))
}

func createExportRequest(name, format, sources string) *http.Request {
	exportRequest := generateExportRequestBody(name, format, sources)
	request, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequest))
	Expect(err).To(BeNil())
	request.Header.Set("Content-Type", "application/json")
	return request
}

var _ = Describe("The public API", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	DescribeTable("can create a new export request", func(name, format, sources, expectedBody string, expectedStatus int) {
		router := setupTest(mockReqeustApplicationResouces)

		rr := httptest.NewRecorder()

		req := createExportRequest(name, format, sources)
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
		router := setupTest(mockReqeustApplicationResouces)

		rr := httptest.NewRecorder()

		// Generate 3 export requests
		for i := 1; i <= 3; i++ {
			req := createExportRequest(
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

	FDescribeTable("can filter and list export requests", func(filter, expectedBody string, expectedStatus int) {
		router := setupTest(mockReqeustApplicationResouces)

		rr := httptest.NewRecorder()

		// Generate 3 export requests
		for i := 1; i <= 3; i++ {
			req := createExportRequest(
				fmt.Sprintf("Test Export Request %d", i),
				"json",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)

			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
		}

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?%s", filter), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(expectedStatus))
		Expect(rr.Body.String()).To(ContainSubstring(expectedBody))
	},
		Entry("by name", "name=Test Export Request 1", "Test Export Request 1", http.StatusAccepted),
		Entry("by status", "status=complete", "Test Export Request 1", http.StatusAccepted),
		Entry("by created at (given date)", "created=2021-01-01T00:00:00Z", "Test Export Request 1", http.StatusAccepted),
		Entry("by created at (given date-time)", "created=2021-01-01T00:00:00Z", "Test Export Request 1", http.StatusAccepted),
		FEntry("by improper created at", "created=spring", "", http.StatusBadRequest),
		Entry("by expires", "expires=2023-01-01T00:00:00Z", "Test Export Request 1", http.StatusAccepted),
		Entry("by improper expires", "expires=nextyear", "", http.StatusBadRequest),
	)

	Describe("can filter exports by date", func() {
		It("with created at in date format", func() {
			router := populateTestData()

			rr := httptest.NewRecorder()

			today := time.Now().Format("2006-01-02")

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created=%s", today), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
		})

		It("with created at in date-time format", func() {
			router := setupTest(mockReqeustApplicationResouces)

			rr := httptest.NewRecorder()

			today := time.Now().Format(time.RFC3339)

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created=%s", today), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
		})

		It("with created at referring to yesterday", func() {
			router := setupTest(mockReqeustApplicationResouces)

			rr := httptest.NewRecorder()

			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created=%s", yesterday), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 3"))
		})
	})

	It("can check the status of an export request", func() {
		router := setupTest(mockReqeustApplicationResouces)

		rr := httptest.NewRecorder()

		req := createExportRequest(
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

	It("sends a request message to the export sources", func() {
		var wasKafkaMessageSent bool

		mockKafkaCall := func(ctx context.Context, identity string, payload models.ExportPayload) {
			wasKafkaMessageSent = true
		}

		router := setupTest(mockKafkaCall)

		rr := httptest.NewRecorder()

		req := createExportRequest(
			"Test Export Request",
			"json",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)

		router.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusAccepted))
		Expect(wasKafkaMessageSent).To(BeTrue())
	})

	// It("can get a completed export request by ID and download it")

	It("can delete a specific export request by ID", func() {
		router := setupTest(mockReqeustApplicationResouces)

		rr := httptest.NewRecorder()

		req := createExportRequest(
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

		// Delete the export request
		req, err = http.NewRequest("DELETE", fmt.Sprintf("/api/export/v1/exports/%s", exportUUID), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		router.ServeHTTP(rr, req)
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

func mockReqeustApplicationResouces(ctx context.Context, identity string, payload models.ExportPayload) {
	fmt.Println("MOCKED !!  KAFKA SENT: TRUE ")
}

func setupTest(requestAppResources exports.RequestApplicationResources) chi.Router {
	var exportHandler *exports.Export
	var router *chi.Mux

	fmt.Println("STARTING TEST")

	exportHandler = &exports.Export{
		Bucket:              "cfg.StorageConfig.Bucket",
		StorageHandler:      &es3.MockStorageHandler{},
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

	fmt.Println("...CLEANING DB...")
	testGormDB.Exec("DELETE FROM export_payloads")

	return router
}

func populateTestData() chi.Router {
	// define router
	router := setupTest(mockReqeustApplicationResouces)

	for i := 1; i <= 3; i++ {
		req := createExportRequest(
			fmt.Sprintf("Test Export Request %d", i),
			"json",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)

		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)
	}

	return router
}
