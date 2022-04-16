package middleware

import (
	"net/http"

	"github.com/redhatinsights/export-service-go/errors"
)

// SliceContainsString returns true if the specified target is present in the given slice.
func SliceContainsString(slice []string, target string) bool {
	for _, element := range slice {
		if element == target {
			return true
		}
	}
	return false
}

func EnforcePSK(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		psk := r.Header["X-Rh-Exports-Psk"]

		if len(psk) != 1 {
			errors.BadRequestError(w, "missing x-rh-exports-psk header")
			return
		}

		if !SliceContainsString(cfg.Psks, psk[0]) {
			errors.JSONError(w, "invalid x-rh-exports-psk header", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
