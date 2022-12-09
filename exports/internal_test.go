package exports_test

import (
	"fmt"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	"github.com/redhatinsights/platform-go-middlewares/identity"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	"github.com/redhatinsights/export-service-go/s3"
)

var _ = Context("Set up internal handler", func() {
	cfg := config.ExportCfg
	cfg.Debug = true

	var internalHandler *exports.Internal
	var router *chi.Mux

	BeforeEach(func() {
		internalHandler = &exports.Internal{
			Cfg: cfg,
			Compressor: &s3.Compressor{
				Bucket: cfg.StorageConfig.Bucket,
				Log:    logger.Log,
			},
			DB:  &models.ExportDB{DB: testGormDB},
			Log: logger.Log,
		}

		router = chi.NewRouter()
		router.Use(
			emiddleware.InjectDebugUserIdentity,
			identity.EnforceIdentity,
			emiddleware.EnforceUserIdentity,
		)

		router.Route("/app/export/v1", func(sub chi.Router) {
			sub.Post("/upload/{id}/{application}/{resource}", internalHandler.PostUpload)
			sub.Post("/error/{id}/{application}/{resource}", internalHandler.PostError)
		})
	})

	Describe("The internal API", func() {
		BeforeEach(func() {
			fmt.Println("...CLEANING DB...")
			testGormDB.Exec("DELETE FROM export_payloads")
		})

		// TODO: Implement these tests
		It("allows the user to upload a payload", func() {})
		It("allows the user to return an error when the export request is invalid", func() {})
	})
})
