/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// EnforcePrivateAuth creates a flexible authentication middleware for the private server
// that supports both PSK (via x-rh-exports-psk header) and OIDC (via Bearer token).
// This allows for gradual migration from PSK to OIDC authentication.
//
// Authentication order:
// 1. If Bearer token present, try OIDC authentication
// 2. If OIDC auth fails or no Bearer token, fall back to PSK authentication
// 3. If both fail, return 401 Unauthorized
func EnforcePrivateAuth(oidcVerifier Verifier, psks []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try OIDC authentication first if Bearer token is present
			if oidcVerifier != nil {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
					if authenticateWithOIDC(r, w, oidcVerifier) {
						next.ServeHTTP(w, r)
						return
					}
					// OIDC failed, but Bearer token was present - don't fall back to PSK
					return
				}
			}

			// Fall back to PSK authentication
			authenticateWithPSK(w, r, psks, next)
		})
	}
}

// authenticateWithOIDC attempts OIDC authentication.
// Returns true if authentication succeeded, false if it failed (error response written).
func authenticateWithOIDC(r *http.Request, w http.ResponseWriter, verifier Verifier) bool {
	authHeader := r.Header.Get("Authorization")
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")

	if rawToken == "" {
		JSONError(w, "empty bearer token", http.StatusUnauthorized)
		return false
	}

	_, err := verifier.Verify(r.Context(), rawToken)
	if err != nil {
		JSONError(w, fmt.Sprintf("OIDC authentication failed: %v", err), http.StatusUnauthorized)
		return false
	}

	return true
}

// authenticateWithPSK performs PSK authentication.
// If successful, calls next handler. If failed, writes error response.
func authenticateWithPSK(w http.ResponseWriter, r *http.Request, psks []string, next http.Handler) {
	psk := r.Header["X-Rh-Exports-Psk"]

	if len(psk) != 1 {
		BadRequestError(w, "missing x-rh-exports-psk header")
		return
	}

	if !SliceContainsString(psks, psk[0]) {
		JSONError(w, "invalid x-rh-exports-psk header", http.StatusUnauthorized)
		return
	}

	next.ServeHTTP(w, r)
}

// EnforceOIDCOnly creates an authentication middleware that only accepts OIDC tokens.
// This can be used when PSK support is fully removed.
func EnforceOIDCOnly(oidcVerifier Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if oidcVerifier == nil {
				JSONError(w, "OIDC authentication is not configured", http.StatusInternalServerError)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				JSONError(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			rawToken := strings.TrimPrefix(authHeader, "Bearer ")
			if rawToken == "" {
				JSONError(w, "empty bearer token", http.StatusUnauthorized)
				return
			}

			_, err := oidcVerifier.Verify(context.Background(), rawToken)
			if err != nil {
				JSONError(w, fmt.Sprintf("OIDC authentication failed: %v", err), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
