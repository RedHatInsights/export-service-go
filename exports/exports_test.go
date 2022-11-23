package exports_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/go-chi/chi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

func CreateTestDB(cfg config.ExportConfig) (*embeddedpostgres.EmbeddedPostgres, *gorm.DB, error) {
	dbStartTime := time.Now()
	fmt.Println("STARTING TEST DB...")

	var db = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(5432).Logger(nil))
	if err := db.Start(); err != nil {
		fmt.Println("Error starting embedded postgres: ", err)
		return nil, nil, err
	}

	dsn := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("Error connecting to embedded postgres: ", err)
		return nil, nil, err
	}

	if err := gdb.AutoMigrate(&models.ExportPayload{}); err != nil {
		fmt.Println("failed to migrate db", "error", err)
		return nil, nil, err
	}

	fmt.Println("TEST DB STARTED IN: ", time.Since(dbStartTime))
	return db, gdb, nil
}

func GenerateExportRequest(name string, format string, sources string) (exportRequest []byte) {
	return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "sources": [%s]}`, name, format, sources))
}

var _ = Context("Set up test DB", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	_, gdb, err := CreateTestDB(*cfg)
	if err != nil {
		fmt.Println("Error creating test DB: ", err)
		panic(err)
	}

	exportHandler := &exports.Export{
		Bucket:    cfg.StorageConfig.Bucket,
		Client:    es3.Client,
		DB:        &models.ExportDB{DB: gdb},
		KafkaChan: make(chan *kafka.Message),
		Log:       logger.Log,
	}

	router := chi.NewRouter()
	router.Use(
		emiddleware.InjectDebugUserIdentity,
		identity.EnforceIdentity,
		emiddleware.EnforceUserIdentity,
	)

	router.Route("/api/export/v1", func(sub chi.Router) {
		sub.Post("/exports", exportHandler.PostExport)
	})

	AfterEach(func() {
		// TODO: Fix this so that the db is cleaned up after all tests complete
		// Currently manually closing the DB using `fuser -k 5432/tcp` after tests complete
		//
		// err := db.Stop()
		// if err != nil {
		// 	fmt.Println("Error stopping embedded postgres: ", err)
		// }
		// fmt.Println("...STOPPED TEST DB")
	})

	Describe("The public API", func() {

		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			gdb.Exec("DELETE FROM export_payloads")
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
			Entry("with an invalid format", "Test Export Request", "abcde", `{"application":"exampleApp", "resource":"exampleResource", "expires":"2023-01-01T00:00:00Z"}`, "Invalid format", http.StatusBadRequest),
			Entry("With no sources", "Test Export Request", "json", "", "No sources provided", http.StatusBadRequest),
		)
		// It("can list all export requests")
		// It("can check the status of an export request")
		// It("can send kafka messages to the export sources")
		// It("can get a specific export request by ID and download the file")
		// It("can delete a specific export request by ID")
	})
})
