package exports_test

import (
	"bytes"
	"encoding/json"
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

var _ = Context("Set up test DB", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	db, gdb, err := CreateTestDB(*cfg)
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

	AfterEach(func() {
		err := db.Stop()
		if err != nil {
			fmt.Println("Error stopping embedded postgres: ", err)
		}
		fmt.Println("...STOPPED TEST DB")
	})

	Describe("The public API", func() {

		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			gdb.Exec("DELETE FROM export_payloads")
		})

		It("can create a new export request", func() {

			exportRequest := models.ExportPayload{
				Name:   "Test Export Request",
				Format: "json",
			}
			exportRequestJson, err := json.Marshal(exportRequest)
			Expect(err).ShouldNot(HaveOccurred())

			req, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequestJson))
			req.Header.Set("Content-Type", "application/json")
			Expect(err).ShouldNot(HaveOccurred())

			handler := http.HandlerFunc(exportHandler.PostExport)
			router.Route("/api/export/v1", func(sub chi.Router) {
				sub.Post("/exports", handler)
			})

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			fmt.Println("Body: ", rr.Body.String())
			Expect(rr.Code).To(Equal(http.StatusAccepted))
		})
		// It("can list all export requests")
		// It("can check the status of an export request")
		// It("can send kafka messages to the export sources")
		// It("can get a specific export request by ID and download the file")
		// It("can delete a specific export request by ID")
	})
})
