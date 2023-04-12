/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/redhatinsights/platform-go-middlewares/identity"
)

type userIdentityKey int

const (
	UserIdentityKey userIdentityKey = iota
)

type User struct {
	AccountID      string
	OrganizationID string
	Username       string
}

// EnforeUserIdentity is a middleware that checks for a valid x-rh-identity
// header and adds the id to the request context.
func EnforceUserIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := identity.Get(r.Context())

		if id.Identity.Type != "User" {
			BadRequestError(w, fmt.Sprintf("'%s' is not a valid user type", id.Identity.Type))
			return
		}

		user := User{
			AccountID:      id.Identity.AccountNumber,
			OrganizationID: id.Identity.OrgID,
			Username:       id.Identity.User.Username,
		}

		ctx := context.WithValue(r.Context(), UserIdentityKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIdentity is a helper function that return the x-rh-identity
// stored in the request context.
func GetUserIdentity(ctx context.Context) User {
	return ctx.Value(UserIdentityKey).(User)
}
