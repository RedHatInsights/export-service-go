//go:build sql
// +build sql

package models_test

import (
	"fmt"
	"time"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/models"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var _ = Describe("Purge expired exports", func() {

	var (
		dbConnection *gorm.DB
	)

	BeforeEach(func() {
		var err error
		dbConnection, err = db.OpenDB(*config.ExportCfg)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("Test that exports are purged correctly",
		func(expiresAtTimestamp time.Time, numberOfRowsToCleanup int) {

			sourceID := uuid.New()
			sourcesJson := fmt.Sprintf("[{\"id\": \"%s\"}]", sourceID)

			export := models.ExportPayload{
				Expires: &expiresAtTimestamp,
				Sources: datatypes.JSON([]byte(sourcesJson)),
			}

			err := dbConnection.Create(&export).Error
			Expect(err).NotTo(HaveOccurred())

			exportDB := models.ExportDB{
				DB:  dbConnection,
				Cfg: config.ExportCfg,
			}

			err = exportDB.DeleteExpiredExports()
			Expect(err).NotTo(HaveOccurred())

			// Attempt to delete the record that we inserted before using the id
			result := dbConnection.Where(&models.ExportPayload{ID: export.ID}).Delete(&models.ExportPayload{})

			// Check that the number of rows that were deleted matches the number
			// of rows we expected to delete
			Expect(int(result.RowsAffected)).To(Equal(numberOfRowsToCleanup))
		},
		Entry("Export record should be purged", time.Now().AddDate(0, 0, -18), 0),
		Entry("Export record should NOT be purged", time.Now(), 1),
	)
})
