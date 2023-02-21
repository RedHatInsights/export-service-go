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

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/logger"
	//"github.com/redhatinsights/export-service-go/models"
)

type userIdentityKey int

//type PayloadFormat string

const (
	UserIdentityKey userIdentityKey = iota
	debugHeader     string          = "eyJpZGVudGl0eSI6eyJhY2NvdW50X251bWJlciI6IjEwMDAxIiwib3JnX2lkIjoiMTAwMDAwMDEiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiIxMDAwMDAwMSJ9LCJ0eXBlIjoiVXNlciIsInVzZXIiOnsidXNlcm5hbWUiOiJ1c2VyX2RldiJ9fX0K"

//	JSON            PayloadFormat   = "json"
)

var (
	Cfg = config.ExportCfg
	log = logger.Log
)

type User struct {
	AccountID      string
	OrganizationID string
	Username       string
}

// InjectDebugUserIdentity is a middleware that set a valid x-rh-identity header
// when operating in DEBUG mode. ** Only used during testing.
func InjectDebugUserIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Cfg.Debug {
			rawHeaders := r.Header["X-Rh-Identity"]

			// request does not have the x-rh-id header
			if len(rawHeaders) != 1 {

				r.Header["X-Rh-Identity"] = []string{debugHeader}
				log.Info("injecting debug header")
			}
		}

		next.ServeHTTP(w, r)
	})
}

// EnforeUserIdentity is a middleware that checks for a valid x-rh-identity
// header and adds the id to the request context.
func EnforceUserIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := identity.Get(r.Context())

		if id.Identity.Type != "User" {
			errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid user type", id.Identity.Type))
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
