package exports_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-chi/chi"
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

func GenerateExportRequest(name string, format string, sources string) (exportRequest []byte) {
	return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "sources": [%s]}`, name, format, sources))
}

var _ = Context("Set up export handler", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	var exportHandler *exports.Export
	var router *chi.Mux

	BeforeEach(func() {
		exportHandler = &exports.Export{
			Bucket:    cfg.StorageConfig.Bucket,
			Client:    es3.Client,
			DB:        &models.ExportDB{DB: testGormDB},
			KafkaChan: make(chan *kafka.Message),
			Log:       logger.Log,
		}

		router = chi.NewRouter()
		router.Use(
			emiddleware.InjectDebugUserIdentity,
			identity.EnforceIdentity,
			emiddleware.EnforceUserIdentity,
		)

		router.Route("/api/export/v1", func(sub chi.Router) {
			sub.Post("/exports", exportHandler.PostExport)
		})
	})

	Describe("The public API", func() {

		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			testGormDB.Exec("DELETE FROM export_payloads")
		})

		DescribeTable("can create a new export request", func(name, format, sources, expectedBody string, expectedStatus int) {

			exportRequestJson := GenerateExportRequest(
				name,
				format,
				sources,
			)

			rr := httptest.NewRecorder()

			req, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequestJson))
			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			router.ServeHTTP(rr, req)

			fmt.Println("Body: ", rr.Body.String())
			Expect(rr.Body.String()).To(ContainSubstring(expectedBody))
			Expect(rr.Code).To(Equal(expectedStatus))
		},
			Entry("with valid request", "Test Export Request", "json", `{"application":"exampleApp", "resource":"exampleResource", "expires":"2023-01-01T00:00:00Z"}`, "", http.StatusAccepted),
			Entry("with no expiration", "Test Export Request", "json", `{"application":"exampleApp", "resource":"exampleResource"}`, "", http.StatusAccepted),
			Entry("with an invalid format", "Test Export Request", "abcde", `{"application":"exampleApp", "resource":"exampleResource", "expires":"2023-01-01T00:00:00Z"}`, "unknown payload format", http.StatusBadRequest),
			Entry("With no sources", "Test Export Request", "json", "", "no sources provided", http.StatusBadRequest),
		)
		// It("can list all export requests")
		// It("can check the status of an export request")
		// It("can send kafka messages to the export sources")
		// It("can get a specific export request by ID and download the file")
		// It("can delete a specific export request by ID")
	})
})
