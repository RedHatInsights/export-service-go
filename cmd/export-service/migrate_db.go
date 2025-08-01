package main

import (
	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"

	"go.uber.org/zap"
)

func performDbMigration(cfg *config.ExportConfig, log *zap.SugaredLogger, direction string) error {
	databaseConn, err := db.OpenPostgresDB(*cfg)
	if err != nil {
		log.Error("Unable to initialize database connection", "error", err)
		return err
	}

	return db.PerformDbMigration(databaseConn, log, "file://db/migrations", direction)
}
