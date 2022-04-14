package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/redhatinsights/platform-go-middlewares/identity"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/models"
)

type userIdentityKey int

const UserIdentityKey userIdentityKey = iota
const debugHeader string = "eyJpZGVudGl0eSI6eyJhY2NvdW50X251bWJlciI6IjEwMDAxIiwib3JnX2lkIjoiMTAwMDAwMDEiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiIxMDAwMDAwMSJ9LCJ0eXBlIjoiVXNlciIsInVzZXIiOnsidXNlcm5hbWUiOiJ1c2VyX2RldiJ9fX0K"

var cfg = config.ExportCfg
var log = logger.Log

func InjectDebugUserIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Auth {
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

func EnforceUserIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := identity.Get(r.Context())

		if id.Identity.Type != "User" {
			errors.JSONError(w, fmt.Sprintf("'%s' is not a valid user type", id.Identity.Type), http.StatusBadRequest)
			return
		}

		user := models.User{
			AccountID:      id.Identity.AccountNumber,
			OrganizationID: id.Identity.OrgID,
			Username:       id.Identity.User.Username,
		}

		ctx := context.WithValue(r.Context(), UserIdentityKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserIdentity(ctx context.Context) models.User {
	return ctx.Value(UserIdentityKey).(models.User)
}
