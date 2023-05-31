package models_test

import (
	"fmt"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/models"
	"github.com/redhatinsights/export-service-go/utils"
)

var (
	testDB     *embeddedpostgres.EmbeddedPostgres
	testGormDB *gorm.DB
	exportDB	*models.ExportDB
)

func TestModels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Models Suite")
}


var _ = BeforeSuite(func() {
	cfg := config.Get()

	var err error
	testDB, testGormDB, err = utils.CreateTestDB(*cfg)

	exportDB = &models.ExportDB{DB: testGormDB, Cfg: cfg}
	Expect(err).To(BeNil())
})

var _ = AfterSuite(func() {
	err := testDB.Stop()
	Expect(err).To(BeNil())
	fmt.Println("TEST DB STOPPED")
})

func setupTest(testGormDB *gorm.DB) {
	fmt.Println("STARTING TEST")
	fmt.Println("...CLEANING DB...")
	testGormDB.Exec("DELETE FROM export_payloads")
}
