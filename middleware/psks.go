/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

import (
	"net/http"

	"github.com/redhatinsights/export-service-go/config"
)

var Cfg = config.Get()

// SliceContainsString returns true if the specified target is present in the given slice.
// TODO: if this function is needed elsewhere, it should be moved to a separate package.
func SliceContainsString(slice []string, target string) bool {
	for _, element := range slice {
		if element == target {
			return true
		}
	}
	return false
}

// EnforcePSK is a middleware that checks for a valid x-rh-exports-psk header
// against a flat list of PSKs. Any valid PSK grants access regardless of application.
func EnforcePSK(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		psk := r.Header["X-Rh-Exports-Psk"]

		if len(psk) != 1 {
			BadRequestError(w, "missing x-rh-exports-psk header")
			return
		}

		if !SliceContainsString(Cfg.Psks, psk[0]) {
			JSONError(w, "invalid x-rh-exports-psk header", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// EnforceAppBoundPSK is a middleware that validates the x-rh-exports-psk header
// against an application-specific PSK. It must run after URLParamsCtx so the
// application name is available in the request context.
func EnforceAppBoundPSK(pskMap map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			psk := r.Header["X-Rh-Exports-Psk"]

			if len(psk) != 1 {
				BadRequestError(w, "missing x-rh-exports-psk header")
				return
			}

			params := GetURLParams(r.Context())
			if params == nil {
				JSONError(w, "unable to determine application context", http.StatusInternalServerError)
				return
			}

			expectedPSK, ok := pskMap[params.Application]
			if !ok {
				JSONError(w, "no PSK configured for application", http.StatusForbidden)
				return
			}

			if psk[0] != expectedPSK {
				JSONError(w, "invalid x-rh-exports-psk header for application", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
