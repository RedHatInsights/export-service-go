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
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/models"
)

// DB is a global variable containing the gorm.DB
var DB *gorm.DB

var (
	cfg = config.ExportCfg
	log = logger.Log
)

func init() {
	dbcfg := cfg.DBConfig
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", dbcfg.User, dbcfg.Password, dbcfg.Hostname, dbcfg.Port, dbcfg.Name, dbcfg.SSLCfg.SSLMode)
	if dbcfg.SSLCfg.RdsCa != nil && *dbcfg.SSLCfg.RdsCa != "" {
		dsn += fmt.Sprintf("&sslrootcert=%s", *dbcfg.SSLCfg.RdsCa)
	}
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	var greeting string
	DB.Raw("select 'Hello, from Postgres!!'").Scan(&greeting)
	log.Info(greeting)

	// all models go here for migration
	if err := DB.AutoMigrate(&models.ExportPayload{}); err != nil {
		log.Panicw("failed to migrate db", "error", err)
	}
}
