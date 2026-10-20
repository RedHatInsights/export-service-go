package db

import (
	"database/sql"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

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

func PerformDbMigration(databaseConn *sql.DB, log *zap.SugaredLogger, pathToMigrationFiles string, direction string) error {
	log.Info("Starting Export Service DB migration")

	driver, err := postgres.WithInstance(databaseConn, &postgres.Config{})
	if err != nil {
		log.Error("Unable to get postgres driver from database connection", "error", err)
		return err
	}

	m, err := migrate.NewWithDatabaseInstance(pathToMigrationFiles, "postgres", driver)
	if err != nil {
		log.Error("Unable to intialize database migration util", "error", err)
		return err
	}

	m.Log = loggerWrapper{log}

	switch direction {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	default:
		return errors.New("invalid operation")
	}

	if errors.Is(err, migrate.ErrNoChange) {
		log.Info("DB migration resulted in no changes")
	} else if err != nil {
		log.Error("DB migration resulted in an error", "error", err)
		return err
	}

	return nil
}
