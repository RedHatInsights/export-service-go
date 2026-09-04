package db

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/redhatinsights/export-service-go/config"
)

func newTestDBConfig() config.ExportConfig {
	var cfg config.ExportConfig
	cfg.DBConfig.User = "user"
	cfg.DBConfig.Password = "pass"
	cfg.DBConfig.Hostname = "db.example.com"
	cfg.DBConfig.Port = "5432"
	cfg.DBConfig.Name = "exportsdb"
	cfg.DBConfig.SSLCfg.SSLMode = "disable"
	return cfg
}

func TestBuildPostgresDSNIncludesTimeouts(t *testing.T) {
	cfg := newTestDBConfig()
	cfg.DBConfig.ConnectTimeout = 5 * time.Second
	cfg.DBConfig.StatementTimeout = 30 * time.Second

	dsn := buildPostgresDSN(cfg)

	if !strings.Contains(dsn, "connect_timeout=5") {
		t.Errorf("expected dsn to contain connect_timeout=5, got %q", dsn)
	}
	if !strings.Contains(dsn, "statement_timeout=30000") {
		t.Errorf("expected dsn to contain statement_timeout=30000, got %q", dsn)
	}
}

func TestBuildPostgresDSNOmitsTimeoutsWhenUnset(t *testing.T) {
	cfg := newTestDBConfig()

	dsn := buildPostgresDSN(cfg)

	if strings.Contains(dsn, "connect_timeout") {
		t.Errorf("expected dsn to omit connect_timeout, got %q", dsn)
	}
	if strings.Contains(dsn, "statement_timeout") {
		t.Errorf("expected dsn to omit statement_timeout, got %q", dsn)
	}
}

func TestApplyConnPoolLimits(t *testing.T) {
	cfg := newTestDBConfig()
	cfg.DBConfig.MaxOpenConns = 7
	cfg.DBConfig.MaxIdleConns = 3
	cfg.DBConfig.ConnMaxLifetime = time.Hour
	cfg.DBConfig.ConnMaxIdleTime = time.Minute

	sqlDB, err := sql.Open("postgres", buildPostgresDSN(cfg))
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("sqlDB.Close() failed: %v", err)
		}
	}()

	applyConnPoolLimits(sqlDB, cfg)

	if stats := sqlDB.Stats(); stats.MaxOpenConnections != 7 {
		t.Errorf("MaxOpenConnections = %d, want 7", stats.MaxOpenConnections)
	}
}
