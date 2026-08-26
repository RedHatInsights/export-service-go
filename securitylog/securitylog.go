/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package securitylog

import (
	"go.uber.org/zap"
)

// Principal represents the identity performing a security-relevant action.
type Principal struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Type   string `json:"type"` // "user", "serviceaccount", "system", "anonymous"
}

func principalFields(p Principal) map[string]string {
	return map[string]string{
		"user_id": p.UserID,
		"org_id":  p.OrgID,
		"type":    p.Type,
	}
}

// Log emits a structured security event for CRUD operations (EOI-1).
// All security events include the required fields: action, resource_type,
// resource_id, outcome, and principal per SEC-MON-REQ-1.
func Log(logger *zap.SugaredLogger, action, resourceType, resourceID, outcome string, principal Principal) {
	logger.Infow("security event",
		"security_event", true,
		"action", action,
		"resource_type", resourceType,
		"resource_id", resourceID,
		"outcome", outcome,
		"principal", principalFields(principal),
	)
}

// LogStartup emits a security event for service startup (EOI-5).
func LogStartup(logger *zap.SugaredLogger, outcome string) {
	logger.Infow("security event",
		"security_event", true,
		"action", "STARTUP",
		"resource_type", "service",
		"resource_id", "export-service",
		"outcome", outcome,
		"principal", map[string]string{"type": "system"},
	)
}

// LogShutdown emits a security event for service shutdown (EOI-5).
// Uses Error level for failure outcomes, Info for success.
// The reason field is omitted when empty.
func LogShutdown(logger *zap.SugaredLogger, outcome, reason string) {
	fields := []interface{}{
		"security_event", true,
		"action", "SHUTDOWN",
		"resource_type", "service",
		"resource_id", "export-service",
		"outcome", outcome,
		"principal", map[string]string{"type": "system"},
	}
	if reason != "" {
		fields = append(fields, "reason", reason)
	}
	if outcome == "failure" {
		logger.Errorw("security event", fields...)
	} else {
		logger.Infow("security event", fields...)
	}
}

// LogAuthFailure emits a security event for authentication or authorization
// failures (EOI-7). The principal is anonymous because the identity could not
// be verified.
func LogAuthFailure(logger *zap.SugaredLogger, method, path string, statusCode int) {
	logger.Warnw("security event",
		"security_event", true,
		"action", "AUTH_FAILURE",
		"resource_type", "endpoint",
		"resource_id", path,
		"outcome", "failure",
		"principal", map[string]string{"type": "anonymous"},
		"method", method,
		"status_code", statusCode,
	)
}
