package main

import (
	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/models"

	"go.uber.org/zap"
)

func startExpiredExportCleaner(cfg *config.ExportConfig, log *zap.SugaredLogger) {
	log.Info("Starting expired export cleaner")

	dbConnection, err := db.OpenDB(*cfg)
	if err != nil {
		log.Panic("failed to open database", "error", err)
	}

	exportsDB := models.ExportDB{
		DB:  dbConnection,
		Cfg: cfg,
	}

	err = exportsDB.DeleteExpiredExports()
	if err != nil {
		log.Error("Expired export cleaner failed", "error", err)
	}
}
