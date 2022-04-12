package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/maskarb/export-service-go/config"
	"github.com/maskarb/export-service-go/logging"
	"github.com/maskarb/export-service-go/models"
)

var DB *gorm.DB
var cfg = config.ExportCfg.DBConfig
var log = logging.Log

func init() {
	dburl := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", cfg.User, cfg.Password, cfg.Hostname, cfg.Port, cfg.Name)
	var err error
	DB, err = gorm.Open(postgres.Open(dburl), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	var greeting string
	DB.Raw("select 'Hello, from Postgres!!'").Scan(&greeting)
	log.Infof(greeting)

	// all models go here for migration
	DB.AutoMigrate(&models.ExportPayload{})
}
