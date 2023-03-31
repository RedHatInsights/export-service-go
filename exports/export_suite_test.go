package exports_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/config"
	db_utils "github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/logger"
)

var (
	testDB     *embeddedpostgres.EmbeddedPostgres
	testGormDB *gorm.DB
)

func TestExports(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exports Suite")
}

func CreateTestDB(cfg config.ExportConfig) (*embeddedpostgres.EmbeddedPostgres, *gorm.DB, error) {
	dbStartTime := time.Now()
	fmt.Println("STARTING TEST DB...")

	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(5432).Logger(nil))
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

	dbConn, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("Error connecting to postgres connection for running migrations: ", err)
		return nil, nil, err
	}

	err = db_utils.PerformDbMigration(dbConn, logger.Log, "file://../db/migrations", "up")
	if err != nil {
		fmt.Println("Database migration failed: ", err)
		return nil, nil, err
	}

	fmt.Println("TEST DB STARTED IN: ", time.Since(dbStartTime))
	return db, gdb, nil
}

var _ = BeforeSuite(func() {
	cfg := config.Get()

	var err error
	testDB, testGormDB, err = CreateTestDB(*cfg)
	Expect(err).To(BeNil())
})

var _ = AfterSuite(func() {
	err := testDB.Stop()
	Expect(err).To(BeNil())
	fmt.Println("TEST DB STOPPED")
})
