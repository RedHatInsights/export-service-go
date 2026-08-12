/*
Copyright 2026 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/

// Package securitylog provides structured security event logging
// per SEC-MON-REQ-1 (Events of Interest) compliance requirements.
//
// All security events include the required fields: action, resource_type,
// resource_id, outcome, and principal information. Events are emitted as
// structured JSON with a "security_event": true marker for log aggregation.
//
// Applicable EOI categories for the Export Service:
//   - EOI-1: CRUD operations on customer data (exports)
//   - EOI-5: Process lifecycle (startup/shutdown)
//   - EOI-7: Authentication failures
//
// Not applicable:
//   - EOI-4: No RBAC/authorization layer (access is identity-based)
//   - EOI-6: Authentication is handled upstream (3scale/SSO)
//   - EOI-8: No fine-grained authorization decisions
//   - EOI-9/EOI-10: Not applicable to this service
package securitylog

import (
	"go.uber.org/zap"
)

// Principal represents the authenticated entity performing the action.
type Principal struct {
	UserID string
	OrgID  string
	Type   string // "user", "serviceaccount", "system"
}

// Event represents a security event with all required SEC-MON-REQ-1 fields.
type Event struct {
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string // "success" or "failure"
	Principal    Principal
	Reason       string // optional: error/failure reason
}

// Log emits a structured security event log entry with all required fields.
func Log(logger *zap.SugaredLogger, event Event) {
	fields := []interface{}{
		"security_event", true,
		"action", event.Action,
		"resource_type", event.ResourceType,
		"resource_id", event.ResourceID,
		"outcome", event.Outcome,
		"principal_user_id", event.Principal.UserID,
		"principal_org_id", event.Principal.OrgID,
		"principal_type", event.Principal.Type,
	}
	if event.Reason != "" {
		fields = append(fields, "reason", event.Reason)
	}

	if event.Outcome == "failure" {
		logger.Warnw("security_event", fields...)
	} else {
		logger.Infow("security_event", fields...)
	}
}

// LogStartup logs a process startup security event (EOI-5).
func LogStartup(logger *zap.SugaredLogger) {
	logger.Infow("security_event",
		"security_event", true,
		"action", "STARTUP",
		"resource_type", "process",
		"resource_id", "export-service",
		"outcome", "success",
	)
}

// LogShutdown logs a process shutdown security event (EOI-5).
// Use outcome "success" for graceful shutdown, "failure" for crashes.
func LogShutdown(logger *zap.SugaredLogger, outcome string, reason string) {
	fields := []interface{}{
		"security_event", true,
		"action", "SHUTDOWN",
		"resource_type", "process",
		"resource_id", "export-service",
		"outcome", outcome,
	}
	if reason != "" {
		fields = append(fields, "reason", reason)
	}

	if outcome == "failure" {
		logger.Errorw("security_event", fields...)
	} else {
		logger.Infow("security_event", fields...)
	}
}

// LogAuthFailure logs an authentication failure security event (EOI-7).
func LogAuthFailure(logger *zap.SugaredLogger, method string, path string, reason string) {
	logger.Warnw("security_event",
		"security_event", true,
		"action", "AUTH_FAILURE",
		"resource_type", "api",
		"resource_id", path,
		"outcome", "failure",
		"method", method,
		"reason", reason,
	)
}
