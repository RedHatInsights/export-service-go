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

type userIdentityKey int

const (
	UserIdentityKey    userIdentityKey = iota
	userType                           = "user"
	serviceAccountType                 = "serviceaccount"
	certIdType                         = "system"
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

		username, err := getUsernameFromIdentityHeader(id)
		if err != nil {
			BadRequestError(w, fmt.Errorf("invalid identity header: %w", err))
			return
		}

		user := User{
			AccountID:      id.Identity.AccountNumber,
			OrganizationID: id.Identity.OrgID,
			Username:       username,
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

func getUsernameFromIdentityHeader(id identity.XRHID) (string, error) {
	identityType := strings.ToLower(id.Identity.Type)

	if identityType == userType {
		return verifyUsername(id.Identity.User.Username)
	}

	if identityType == serviceAccountType {
		if id.Identity.ServiceAccount == nil {
			return "", fmt.Errorf("missing ServiceAccount data")
		}
		return verifyUsername(id.Identity.ServiceAccount.Username)
	}

	if identityType == certIdType {
		if id.Identity.System == nil {
			return "", fmt.Errorf("missing cert data")
		}
		return verifyUsername(id.Identity.System.CommonName)
	}

	return "", fmt.Errorf("'%s' is not a valid user type", id.Identity.Type)
}

func verifyUsername(username string) (string, error) {
	// The security model is currently based on the username...so verify we are getting a valid username
	if len(username) == 0 {
		return "", fmt.Errorf("missing username data")
	}

	return username, nil
}
