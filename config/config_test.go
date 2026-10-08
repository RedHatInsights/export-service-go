package config

import (
	"sync"
	"testing"
	"time"
)

// resetConfig resets the singleton so Get() can be called again with different env vars.
func resetConfig() {
	config = nil
	doOnce = sync.Once{}
}

func TestDBConfigTimeoutsAndPool(t *testing.T) {
	t.Setenv("PGSQL_CONNECT_TIMEOUT", "2s")
	t.Setenv("PGSQL_MAX_OPEN_CONNS", "42")

	cfg := Get()

	if cfg.DBConfig.ConnectTimeout != 2*time.Second {
		t.Errorf("ConnectTimeout = %v, want 2s", cfg.DBConfig.ConnectTimeout)
	}
	if cfg.DBConfig.MaxOpenConns != 42 {
		t.Errorf("MaxOpenConns = %d, want 42", cfg.DBConfig.MaxOpenConns)
	}

	// Env vars left unset should fall back to their defaults.
	if cfg.DBConfig.StatementTimeout != 5*time.Second {
		t.Errorf("StatementTimeout = %v, want default 5s", cfg.DBConfig.StatementTimeout)
	}
	if cfg.DBConfig.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want default 5", cfg.DBConfig.MaxIdleConns)
	}
	if cfg.DBConfig.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want default 30m", cfg.DBConfig.ConnMaxLifetime)
	}
	if cfg.DBConfig.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want default 5m", cfg.DBConfig.ConnMaxIdleTime)
	}
}

func TestTimeoutDefaults(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)

	cfg := Get()

	if cfg.StorageConfig.UploadTimeout != 600*time.Second {
		t.Errorf("UploadTimeout = %v, want %v", cfg.StorageConfig.UploadTimeout, 600*time.Second)
	}
	if cfg.StorageConfig.CompressTimeout != 900*time.Second {
		t.Errorf("CompressTimeout = %v, want %v", cfg.StorageConfig.CompressTimeout, 900*time.Second)
	}
}

func TestTimeoutCustomValues(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)

	tests := []struct {
		name         string
		envValue     string
		wantDuration time.Duration
		envKey       string
		getField     func(*ExportConfig) time.Duration
	}{
		{
			name:         "compress timeout 5m",
			envKey:       "S3_COMPRESS_TIMEOUT",
			envValue:     "5m",
			wantDuration: 5 * time.Minute,
			getField:     func(c *ExportConfig) time.Duration { return c.StorageConfig.CompressTimeout },
		},
		{
			name:         "upload timeout 1h30m",
			envKey:       "S3_UPLOAD_TIMEOUT",
			envValue:     "1h30m",
			wantDuration: 90 * time.Minute,
			getField:     func(c *ExportConfig) time.Duration { return c.StorageConfig.UploadTimeout },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConfig()

			t.Setenv(tt.envKey, tt.envValue)

			cfg := Get()
			got := tt.getField(cfg)
			if got != tt.wantDuration {
				t.Errorf("%s = %v, want %v", tt.envKey, got, tt.wantDuration)
			}
		})
	}
}

func TestParsePSKs(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPsks     []string
		wantPskMap   map[string]string
		wantFlatMode bool
	}{
		{
			name:         "empty string",
			input:        "",
			wantPsks:     nil,
			wantPskMap:   map[string]string{},
			wantFlatMode: false,
		},
		{
			name:         "single flat PSK",
			input:        "my-secret-psk",
			wantPsks:     []string{"my-secret-psk"},
			wantFlatMode: true,
		},
		{
			name:         "multiple flat PSKs",
			input:        "psk1,psk2,psk3",
			wantPsks:     []string{"psk1", "psk2", "psk3"},
			wantFlatMode: true,
		},
		{
			name:         "flat PSKs with whitespace",
			input:        " psk1 , psk2 ",
			wantPsks:     []string{"psk1", "psk2"},
			wantFlatMode: true,
		},
		{
			name:  "JSON app-bound PSKs",
			input: `{"subscriptions":"psk-subs","urn:redhat:application:inventory":"psk-inv"}`,
			wantPskMap: map[string]string{
				"subscriptions":                    "psk-subs",
				"urn:redhat:application:inventory": "psk-inv",
			},
			wantFlatMode: false,
		},
		{
			name:         "invalid JSON falls back to flat",
			input:        `{invalid-json`,
			wantPsks:     []string{"{invalid-json"},
			wantFlatMode: true,
		},
		{
			name:         "empty JSON object falls back to flat",
			input:        `{}`,
			wantPsks:     []string{"{}"},
			wantFlatMode: true,
		},
		{
			name:         "trailing commas ignored",
			input:        "psk1,,psk2,",
			wantPsks:     []string{"psk1", "psk2"},
			wantFlatMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psks, pskMap := parsePSKs(tt.input)

			if tt.wantFlatMode {
				if len(pskMap) != 0 {
					t.Errorf("expected empty pskMap in flat mode, got %v", pskMap)
				}
				if len(psks) != len(tt.wantPsks) {
					t.Fatalf("expected %d psks, got %d: %v", len(tt.wantPsks), len(psks), psks)
				}
				for i, want := range tt.wantPsks {
					if psks[i] != want {
						t.Errorf("psks[%d] = %q, want %q", i, psks[i], want)
					}
				}
			} else {
				if psks != nil {
					t.Errorf("expected nil psks in map mode, got %v", psks)
				}
				if len(pskMap) != len(tt.wantPskMap) {
					t.Fatalf("expected %d pskMap entries, got %d: %v", len(tt.wantPskMap), len(pskMap), pskMap)
				}
				for k, want := range tt.wantPskMap {
					got, ok := pskMap[k]
					if !ok {
						t.Errorf("pskMap missing key %q", k)
					} else if got != want {
						t.Errorf("pskMap[%q] = %q, want %q", k, got, want)
					}
				}
			}
		})
	}
}
