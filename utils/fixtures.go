package utils

import (
	"database/sql"
	"fmt"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	db_utils "github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/config"
)


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

	err = db_utils.PerformDbMigration(dbConn, logger.Get(), "file://../db/migrations", "up")
	if err != nil {
		fmt.Println("Database migration failed: ", err)
		return nil, nil, err
	}

	fmt.Println("TEST DB STARTED IN: ", time.Since(dbStartTime))
	return db, gdb, nil
}
