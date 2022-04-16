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

func IsValidUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

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
