package exports_test

import (
	"fmt"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var testDB *embeddedpostgres.EmbeddedPostgres
var testGormDB *gorm.DB

func TestExports(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exports Suite")
}

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

var _ = BeforeSuite(func() {
	cfg := config.ExportCfg

	var err error
	testDB, testGormDB, err = CreateTestDB(*cfg)
	Expect(err).To(BeNil())
})

var _ = AfterSuite(func() {
	err := testDB.Stop()
	Expect(err).To(BeNil())
	fmt.Println("TEST DB STOPPED")
})
