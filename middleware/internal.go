/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package middleware

import (
	"context"
	"fmt"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/redhatinsights/export-service-go/errors"
	"github.com/redhatinsights/export-service-go/models"
)

type internalKey int

const urlParamsKey internalKey = iota

// IsValidUUID is a helper function that checks if the given string is a valid uuid.
func IsValidUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

// URLParamsCtx is a middleware that pulls `exportUUID`, `resourceUUID`, and `application`
// from the url and puts them into a `urlParams` object in the request context.
func URLParamsCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var uid string
		uid = chi.URLParam(r, "exportUUID")
		exportUUID, err := uuid.Parse(uid)
		if err != nil {
			errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", uid))
			return
		}

		uid = chi.URLParam(r, "resourceUUID")
		resourceUUID, err := uuid.Parse(uid)
		if err != nil {
			errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid resource UUID", uid))
		}

		application := chi.URLParam(r, "application")

		params := &models.URLParams{
			ExportUUID:   exportUUID,
			Application:  application,
			ResourceUUID: resourceUUID,
		}

		ctx := context.WithValue(r.Context(), urlParamsKey, params)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetURLParams fetches the urlParams from the context.
func GetURLParams(ctx context.Context) *models.URLParams {
	return ctx.Value(urlParamsKey).(*models.URLParams)
}
