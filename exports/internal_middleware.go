/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redhatinsights/export-service-go/errors"
)

type internalKey int

const urlParamsKey internalKey = iota

// IsValidUUID is a helper function that checks if the given string is a valid uuid.
func IsValidUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

// URLParams is a middleware that pulls `exportUUID`, `resourceUUID`, and `application`
// from the url and puts them into a `urlParams` object in the request context.
func URLParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exportUUID := chi.URLParam(r, "exportUUID")
		if !IsValidUUID(exportUUID) {
			errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid export UUID", exportUUID))
			return
		}

		resourceUUID := chi.URLParam(r, "resourceUUID")
		if !IsValidUUID(resourceUUID) {
			errors.BadRequestError(w, fmt.Sprintf("'%s' is not a valid resource UUID", exportUUID))
		}

		application := chi.URLParam(r, "application")

		params := &urlParams{
			exportUUID:   exportUUID,
			application:  application,
			resourceUUID: resourceUUID,
		}

		ctx := context.WithValue(r.Context(), urlParamsKey, params)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
