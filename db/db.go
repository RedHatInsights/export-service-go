/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/config"
)

func OpenDB(cfg config.ExportConfig) (*gorm.DB, error) {
	dbcfg := cfg.DBConfig
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", dbcfg.User, dbcfg.Password, dbcfg.Hostname, dbcfg.Port, dbcfg.Name, dbcfg.SSLCfg.SSLMode)
	if dbcfg.SSLCfg.RdsCa != nil && *dbcfg.SSLCfg.RdsCa != "" {
		dsn += fmt.Sprintf("&sslrootcert=%s", *dbcfg.SSLCfg.RdsCa)
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
