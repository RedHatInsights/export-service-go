package exports_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/confluentinc/confluent-kafka-go/kafka"
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

var _ = Describe("Public API", func() {
	var exportHandler *exports.Export
	// var mock sqlmock.Sqlmock

	cfg := config.ExportCfg
	cfg.Debug = true

	router := chi.NewRouter()
	router.Use(
		emiddleware.InjectDebugUserIdentity,
		identity.EnforceIdentity,
		emiddleware.EnforceUserIdentity,
	)

	BeforeEach(func() {
		var db *sql.DB
		db, _, err := sqlmock.New()
		Expect(err).ShouldNot(HaveOccurred())

		gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
		Expect(err).ShouldNot(HaveOccurred())

		exportHandler = &exports.Export{
			Bucket:    cfg.StorageConfig.Bucket,
			Client:    es3.Client,
			DB:        &models.ExportDB{DB: gdb},
			KafkaChan: make(chan *kafka.Message),
			Log:       logger.Log,
		}

	})

	It("can create a new export request", func() {

		exportRequest := models.ExportPayload{
			Name:   "Example Export Request",
			Format: "json",
		}
		exportRequestJson, err := json.Marshal(exportRequest)
		Expect(err).ShouldNot(HaveOccurred())

		req, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequestJson))
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		rr := httptest.NewRecorder()

		handler := http.HandlerFunc(exportHandler.PostExport)

		router.Route("/api/export/v1", func(sub chi.Router) {
			sub.Post("/exports", handler)
		})

		router.ServeHTTP(rr, req)

		fmt.Println("Body: ", rr.Body.String())
		Expect(rr.Code).To(Equal(http.StatusOK))
	})
	// It("can list all export requests")
	// It("can check the status of an export request")
	// It("can send kafka messages to the export sources")
	// It("can get a specific export request by ID and download the file")
	// It("can delete a specific export request by ID")
})
