/*
Copyright 2026 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package securitylog_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/redhatinsights/export-service-go/securitylog"
)

// captureOutput creates a zap.SugaredLogger that writes to a buffer
// and returns both the logger and buffer for assertion.
func captureOutput() (*zap.SugaredLogger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = ""
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(buf),
		zapcore.DebugLevel,
	)
	logger := zap.New(core).Sugar()
	return logger, buf
}

// parseLogEntry parses a single JSON log line from the buffer.
func parseLogEntry(buf *bytes.Buffer) map[string]interface{} {
	var entry map[string]interface{}
	_ = json.Unmarshal(buf.Bytes(), &entry)
	return entry
}

func TestLog_Success(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.Log(logger, securitylog.Event{
		Action:       "CREATE",
		ResourceType: "export",
		ResourceID:   "abc-123",
		Outcome:      "success",
		Principal: securitylog.Principal{
			UserID: "user-1",
			OrgID:  "org-1",
			Type:   "user",
		},
	})

	entry := parseLogEntry(buf)
	if entry["security_event"] != true {
		t.Error("expected security_event=true")
	}
	if entry["action"] != "CREATE" {
		t.Errorf("expected action=CREATE, got %v", entry["action"])
	}
	if entry["resource_type"] != "export" {
		t.Errorf("expected resource_type=export, got %v", entry["resource_type"])
	}
	if entry["resource_id"] != "abc-123" {
		t.Errorf("expected resource_id=abc-123, got %v", entry["resource_id"])
	}
	if entry["outcome"] != "success" {
		t.Errorf("expected outcome=success, got %v", entry["outcome"])
	}
	if entry["principal_user_id"] != "user-1" {
		t.Errorf("expected principal_user_id=user-1, got %v", entry["principal_user_id"])
	}
	if entry["principal_org_id"] != "org-1" {
		t.Errorf("expected principal_org_id=org-1, got %v", entry["principal_org_id"])
	}
	if entry["principal_type"] != "user" {
		t.Errorf("expected principal_type=user, got %v", entry["principal_type"])
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info for success, got %v", entry["level"])
	}
	// reason should not be present when empty
	if _, ok := entry["reason"]; ok {
		t.Error("reason should not be present when empty")
	}
}

func TestLog_Failure(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.Log(logger, securitylog.Event{
		Action:       "DELETE",
		ResourceType: "export",
		ResourceID:   "def-456",
		Outcome:      "failure",
		Principal: securitylog.Principal{
			UserID: "user-2",
			OrgID:  "org-2",
			Type:   "serviceaccount",
		},
		Reason: "record not found",
	})

	entry := parseLogEntry(buf)
	if entry["level"] != "warn" {
		t.Errorf("expected level=warn for failure, got %v", entry["level"])
	}
	if entry["reason"] != "record not found" {
		t.Errorf("expected reason='record not found', got %v", entry["reason"])
	}
	if entry["outcome"] != "failure" {
		t.Errorf("expected outcome=failure, got %v", entry["outcome"])
	}
}

func TestLogStartup(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.LogStartup(logger)

	entry := parseLogEntry(buf)
	if entry["security_event"] != true {
		t.Error("expected security_event=true")
	}
	if entry["action"] != "STARTUP" {
		t.Errorf("expected action=STARTUP, got %v", entry["action"])
	}
	if entry["resource_type"] != "process" {
		t.Errorf("expected resource_type=process, got %v", entry["resource_type"])
	}
	if entry["resource_id"] != "export-service" {
		t.Errorf("expected resource_id=export-service, got %v", entry["resource_id"])
	}
	if entry["outcome"] != "success" {
		t.Errorf("expected outcome=success, got %v", entry["outcome"])
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
}

func TestLogShutdown_Success(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.LogShutdown(logger, "success", "")

	entry := parseLogEntry(buf)
	if entry["security_event"] != true {
		t.Error("expected security_event=true")
	}
	if entry["action"] != "SHUTDOWN" {
		t.Errorf("expected action=SHUTDOWN, got %v", entry["action"])
	}
	if entry["outcome"] != "success" {
		t.Errorf("expected outcome=success, got %v", entry["outcome"])
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info for successful shutdown, got %v", entry["level"])
	}
	// reason should not be present when empty
	if _, ok := entry["reason"]; ok {
		t.Error("reason should not be present for successful shutdown")
	}
}

func TestLogShutdown_Failure(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.LogShutdown(logger, "failure", "unexpected panic")

	entry := parseLogEntry(buf)
	if entry["outcome"] != "failure" {
		t.Errorf("expected outcome=failure, got %v", entry["outcome"])
	}
	if entry["level"] != "error" {
		t.Errorf("expected level=error for failed shutdown, got %v", entry["level"])
	}
	if entry["reason"] != "unexpected panic" {
		t.Errorf("expected reason='unexpected panic', got %v", entry["reason"])
	}
}

func TestLogAuthFailure(t *testing.T) {
	logger, buf := captureOutput()

	securitylog.LogAuthFailure(logger, "POST", "/api/export/v1/exports", "unauthorized")

	entry := parseLogEntry(buf)
	if entry["security_event"] != true {
		t.Error("expected security_event=true")
	}
	if entry["action"] != "AUTH_FAILURE" {
		t.Errorf("expected action=AUTH_FAILURE, got %v", entry["action"])
	}
	if entry["resource_type"] != "api" {
		t.Errorf("expected resource_type=api, got %v", entry["resource_type"])
	}
	if entry["resource_id"] != "/api/export/v1/exports" {
		t.Errorf("expected resource_id=/api/export/v1/exports, got %v", entry["resource_id"])
	}
	if entry["outcome"] != "failure" {
		t.Errorf("expected outcome=failure, got %v", entry["outcome"])
	}
	if entry["method"] != "POST" {
		t.Errorf("expected method=POST, got %v", entry["method"])
	}
	if entry["level"] != "warn" {
		t.Errorf("expected level=warn, got %v", entry["level"])
	}
}

func TestAuthFailureMiddleware_MutatingMethod401(t *testing.T) {
	logger, buf := captureOutput()

	handler := securitylog.AuthFailureMiddleware(logger)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/export/v1/exports", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	entry := parseLogEntry(buf)
	if entry["action"] != "AUTH_FAILURE" {
		t.Errorf("expected auth failure log, got %v", entry)
	}
	if entry["reason"] != "unauthorized" {
		t.Errorf("expected reason=unauthorized, got %v", entry["reason"])
	}
}

func TestAuthFailureMiddleware_MutatingMethod403(t *testing.T) {
	logger, buf := captureOutput()

	handler := securitylog.AuthFailureMiddleware(logger)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/export/v1/exports/123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	entry := parseLogEntry(buf)
	if entry["action"] != "AUTH_FAILURE" {
		t.Errorf("expected auth failure log for 403")
	}
	if entry["reason"] != "forbidden" {
		t.Errorf("expected reason=forbidden, got %v", entry["reason"])
	}
}

func TestAuthFailureMiddleware_ReadMethodNoLog(t *testing.T) {
	logger, buf := captureOutput()

	handler := securitylog.AuthFailureMiddleware(logger)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/export/v1/exports", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	// GET is not a mutating method — no security log expected
	if buf.Len() > 0 {
		t.Errorf("expected no log for GET auth failure, got: %s", buf.String())
	}
}

func TestAuthFailureMiddleware_SuccessNoLog(t *testing.T) {
	logger, buf := captureOutput()

	handler := securitylog.AuthFailureMiddleware(logger)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/export/v1/exports", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Successful request — no security log expected
	if buf.Len() > 0 {
		t.Errorf("expected no log for successful POST, got: %s", buf.String())
	}
}
