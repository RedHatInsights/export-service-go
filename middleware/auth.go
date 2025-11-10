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

	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

// EnforceAuthentication creates a unified authentication middleware that supports
// both OIDC Bearer tokens and x-rh-identity headers.
//
// When oidcVerifier is provided (non-nil):
//   - Checks for Authorization: Bearer header first
//   - If Bearer token present, verifies with OIDC
//   - Falls back to x-rh-identity if no Bearer token
//
// When oidcVerifier is nil:
//   - Only uses x-rh-identity authentication
func EnforceAuthentication(oidcVerifier Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try OIDC authentication if enabled and Bearer token present
			if user, ok := tryOIDCAuth(r, w, oidcVerifier); ok {
				ctx := context.WithValue(r.Context(), UserIdentityKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fall back to x-rh-identity authentication
			user, err := authenticateXRHIdentity(r.Context())
			if err != nil {
				BadRequestError(w, fmt.Errorf("authentication failed: %w", err))
				return
			}

			ctx := context.WithValue(r.Context(), UserIdentityKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func tryOIDCAuth(r *http.Request, w http.ResponseWriter, verifier Verifier) (User, bool) {
	if verifier == nil {
		return User{}, false
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return User{}, false
	}

	user, err := authenticateOIDC(r.Context(), verifier, authHeader)
	if err != nil {
		JSONError(w, fmt.Sprintf("OIDC authentication failed: %v", err), http.StatusUnauthorized)
		return User{}, false
	}

	return user, true
}

// authenticateOIDC verifies an OIDC Bearer token and returns the user
func authenticateOIDC(ctx context.Context, verifier Verifier, authHeader string) (User, error) {
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")
	if rawToken == "" {
		return User{}, fmt.Errorf("empty bearer token")
	}

	// Verify token using JWKS
	idToken, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return User{}, err
	}

	// Extract claims
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return User{}, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Build User from OIDC claims
	user := User{
		Username: extractUsernameFromClaims(claims, idToken.Subject),
	}

	if orgID, ok := claims["org_id"].(string); ok {
		user.OrganizationID = orgID
	}
	if accountID, ok := claims["account_id"].(string); ok {
		user.AccountID = accountID
	}

	return user, nil
}

// authenticateXRHIdentity verifies x-rh-identity and returns the user
func authenticateXRHIdentity(ctx context.Context) (User, error) {
	id := identity.Get(ctx)
	if id.Identity.AccountNumber == "" {
		return User{}, fmt.Errorf("missing or invalid x-rh-identity header")
	}

	username, err := getUsernameFromIdentityHeader(id)
	if err != nil {
		return User{}, err
	}

	return User{
		AccountID:      id.Identity.AccountNumber,
		OrganizationID: id.Identity.OrgID,
		Username:       username,
	}, nil
}

// extractUsernameFromClaims gets username from OIDC claims with fallback chain:
// preferred_username -> email -> name -> subject
func extractUsernameFromClaims(claims map[string]interface{}, subject string) string {
	if username, ok := claims["preferred_username"].(string); ok && username != "" {
		return username
	}
	if email, ok := claims["email"].(string); ok && email != "" {
		return email
	}
	if name, ok := claims["name"].(string); ok && name != "" {
		return name
	}
	return subject
}
