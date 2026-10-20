/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package db

import (
	"database/sql"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/config"
)

func OpenDB(cfg config.ExportConfig) (*gorm.DB, error) {
	dsn := buildPostgresDSN(cfg)

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	applyConnPoolLimits(sqlDB, cfg)

	return gdb, nil
}

func OpenPostgresDB(cfg config.ExportConfig) (*sql.DB, error) {
	dsn := buildPostgresDSN(cfg)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	applyConnPoolLimits(sqlDB, cfg)

	return sqlDB, nil
}

// Based on GORM's docs, the pool has to be configured in the underlying *sql.DB object
func applyConnPoolLimits(sqlDB *sql.DB, cfg config.ExportConfig) {
	dbcfg := cfg.DBConfig

	sqlDB.SetMaxOpenConns(dbcfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(dbcfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(dbcfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(dbcfg.ConnMaxIdleTime)
}

func buildPostgresDSN(cfg config.ExportConfig) string {
	dbcfg := cfg.DBConfig

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		dbcfg.User,
		dbcfg.Password,
		dbcfg.Hostname,
		dbcfg.Port,
		dbcfg.Name,
		dbcfg.SSLCfg.SSLMode)

	if dbcfg.SSLCfg.RdsCa != nil && *dbcfg.SSLCfg.RdsCa != "" {
		dsn += fmt.Sprintf("&sslrootcert=%s", *dbcfg.SSLCfg.RdsCa)
	}

	if dbcfg.ConnectTimeout > 0 {
		dsn += fmt.Sprintf("&connect_timeout=%d", int(dbcfg.ConnectTimeout.Seconds()))
	}

	if dbcfg.StatementTimeout > 0 {
		dsn += fmt.Sprintf("&statement_timeout=%d", dbcfg.StatementTimeout.Milliseconds())
	}

	return dsn
}
