/*
Copyright 2026 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package securitylog

import (
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// AuthFailureMiddleware wraps downstream handlers and logs authentication
// failures (HTTP 401/403) as security events. Only mutating methods
// (POST, PUT, PATCH, DELETE) are logged to avoid noise from health probes,
// metrics scrapes, and read-only requests.
func AuthFailureMiddleware(logger *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				reason := "unauthorized"
				if status == http.StatusForbidden {
					reason = "forbidden"
				}
				LogAuthFailure(logger, r.Method, r.URL.Path, reason)
			}
		})
	}
}

// isMutatingMethod returns true for HTTP methods that modify data.
// Auth failure logging is restricted to these methods to avoid noise
// from health probes, metrics endpoints, and read-only operations.
func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
