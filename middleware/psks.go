/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

import (
	"net/http"
)

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

// EnforcePSK is a middleware that checks for a valid x-rh-exports-psk header.
func EnforcePSK(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		psk := r.Header["X-Rh-Exports-Psk"]

		if len(psk) != 1 {
			badRequestError(w, "missing x-rh-exports-psk header")
			return
		}

		if !SliceContainsString(Cfg.Psks, psk[0]) {
			jsonError(w, "invalid x-rh-exports-psk header", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
