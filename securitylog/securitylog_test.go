/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package securitylog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// captureOutput runs fn with a buffered logger and returns the raw JSON output.
func captureOutput(fn func(*zap.SugaredLogger)) string {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	fn(logger)
	_ = logger.Sync()
	return buf.String()
}

// parseLine parses a single JSON log line.
func parseLine(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	line := strings.TrimSpace(raw)
	if line == "" {
		t.Fatal("empty log output")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, line)
	}
	return m
}

func assertField(t *testing.T, m map[string]interface{}, key string, expected interface{}) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("missing field %q", key)
		return
	}
	if got != expected {
		t.Errorf("field %q: got %v (%T), want %v (%T)", key, got, got, expected, expected)
	}
}

func TestLog(t *testing.T) {
	principal := Principal{UserID: "testuser", OrgID: "org123", Type: "user"}
	output := captureOutput(func(l *zap.SugaredLogger) {
		Log(l, "CREATE", "export", "uuid-1", "success", principal)
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "CREATE")
	assertField(t, m, "resource_type", "export")
	assertField(t, m, "resource_id", "uuid-1")
	assertField(t, m, "outcome", "success")

	p, ok := m["principal"].(map[string]interface{})
	if !ok {
		t.Fatal("principal is not a map")
	}
	if p["user_id"] != "testuser" {
		t.Errorf("principal.user_id: got %v, want testuser", p["user_id"])
	}
	if p["org_id"] != "org123" {
		t.Errorf("principal.org_id: got %v, want org123", p["org_id"])
	}
	if p["type"] != "user" {
		t.Errorf("principal.type: got %v, want user", p["type"])
	}
}

func TestLogFailure(t *testing.T) {
	principal := Principal{UserID: "svc", OrgID: "org456", Type: "serviceaccount"}
	output := captureOutput(func(l *zap.SugaredLogger) {
		Log(l, "DELETE", "export", "uuid-2", "failure", principal)
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "DELETE")
	assertField(t, m, "outcome", "failure")
}

func TestLogStartup(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		LogStartup(l, "success")
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "STARTUP")
	assertField(t, m, "resource_type", "service")
	assertField(t, m, "resource_id", "export-service")
	assertField(t, m, "outcome", "success")
	assertField(t, m, "level", "info")
}

func TestLogShutdownSuccess(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		LogShutdown(l, "success", "")
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "SHUTDOWN")
	assertField(t, m, "outcome", "success")
	assertField(t, m, "level", "info")

	if _, ok := m["reason"]; ok {
		t.Error("reason should be omitted when empty")
	}
}

func TestLogShutdownFailure(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		LogShutdown(l, "failure", "unexpected crash")
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "SHUTDOWN")
	assertField(t, m, "outcome", "failure")
	assertField(t, m, "level", "error")
	assertField(t, m, "reason", "unexpected crash")
}

func TestLogAuthFailure(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		LogAuthFailure(l, "POST", "/api/export/v1/exports", 401)
	})

	m := parseLine(t, output)
	assertField(t, m, "security_event", true)
	assertField(t, m, "action", "AUTH_FAILURE")
	assertField(t, m, "resource_type", "endpoint")
	assertField(t, m, "resource_id", "/api/export/v1/exports")
	assertField(t, m, "outcome", "failure")
	assertField(t, m, "level", "warn")
	assertField(t, m, "method", "POST")
	// status_code is a float64 in JSON
	assertField(t, m, "status_code", float64(401))

	p, ok := m["principal"].(map[string]interface{})
	if !ok {
		t.Fatal("principal is not a map")
	}
	if p["type"] != "anonymous" {
		t.Errorf("principal.type: got %v, want anonymous", p["type"])
	}
}

func TestAuthFailureMiddleware_MutatingMethod401(t *testing.T) {
	var logged bool
	output := captureOutput(func(l *zap.SugaredLogger) {
		handler := AuthFailureMiddleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/export/v1/exports", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		logged = true
	})

	if !logged {
		t.Fatal("handler did not execute")
	}

	m := parseLine(t, output)
	assertField(t, m, "action", "AUTH_FAILURE")
	assertField(t, m, "status_code", float64(401))
}

func TestAuthFailureMiddleware_MutatingMethod403(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		handler := AuthFailureMiddleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

		req := httptest.NewRequest(http.MethodDelete, "/api/export/v1/exports/uuid", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})

	m := parseLine(t, output)
	assertField(t, m, "action", "AUTH_FAILURE")
	assertField(t, m, "status_code", float64(403))
}

func TestAuthFailureMiddleware_ReadMethodIgnored(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		handler := AuthFailureMiddleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/export/v1/exports", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})

	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no log output for GET request, got: %s", output)
	}
}

func TestAuthFailureMiddleware_SuccessNoLog(t *testing.T) {
	output := captureOutput(func(l *zap.SugaredLogger) {
		handler := AuthFailureMiddleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/export/v1/exports", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})

	if strings.TrimSpace(output) != "" {
		t.Errorf("expected no log output for 202 response, got: %s", output)
	}
}

func TestIsMutatingMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodGet, false},
		{http.MethodHead, false},
		{http.MethodOptions, false},
		{http.MethodPost, true},
		{http.MethodPut, true},
		{http.MethodPatch, true},
		{http.MethodDelete, true},
	}
	for _, tt := range tests {
		if got := isMutatingMethod(tt.method); got != tt.want {
			t.Errorf("isMutatingMethod(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}
