/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package securitylog

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// isMutatingMethod returns true for HTTP methods that modify resources.
// Auth failure logging is restricted to mutating methods to avoid noise
// from health probes, metrics scrapes, and spec endpoints.
func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// AuthFailureMiddleware detects 401/403 responses from downstream handlers
// (e.g. identity.EnforceIdentity) and emits security events (EOI-7).
// Only logs for mutating HTTP methods to avoid noise from read-only probes.
func AuthFailureMiddleware(logger *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				LogAuthFailure(logger, r.Method, r.URL.Path, status)
			}
		})
	}
}
