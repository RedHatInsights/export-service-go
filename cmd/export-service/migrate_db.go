package main

import (
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"

	"go.uber.org/zap"
)

type loggerWrapper struct {
	*zap.SugaredLogger
}

func (lw loggerWrapper) Verbose() bool {
	return true
}

func (lw loggerWrapper) Printf(format string, v ...interface{}) {
	lw.Infof(format, v...)
}

func performDbMigration(cfg *config.ExportConfig, log *zap.SugaredLogger, direction string) error {

	log.Info("Starting Export Service DB migration")

	db, err := db.OpenPostgresDB(*cfg)
	if err != nil {
		log.Error("Unable to initialize database connection", "error", err)
		return err
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Error("Unable to get postgres driver from database connection", "error", err)
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://db/migrations", "postgres", driver)
	if err != nil {
		log.Error("Unable to intialize database migration util", "error", err)
		return err
	}

	m.Log = loggerWrapper{log}

	if direction == "up" {
		err = m.Up()
	} else if direction == "down" {
		err = m.Steps(-1)
	} else {
		return errors.New("Invalid operation")
	}

	if errors.Is(err, migrate.ErrNoChange) {
		log.Info("DB migration resulted in no changes")
	} else if err != nil {
		log.Error("DB migration resulted in an error", "error", err)
		return err
	}

	return nil
}
