/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

// EnforceAuthentication creates a middleware that authenticates users via x-rh-identity header.
// This middleware is used for the public API and expects identity.EnforceIdentity to have
// already run to extract the x-rh-identity header into the context.
//
// Example usage:
//
//	router.Use(identity.EnforceIdentity)  // Extract x-rh-identity
//	router.Use(middleware.EnforceAuthentication())  // Validate and convert to User
func EnforceAuthentication() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// authenticateXRHIdentity extracts user info from x-rh-identity context
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
