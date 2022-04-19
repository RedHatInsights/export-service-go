/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package middleware

import "net/http"

// SetContentType is a middleware that sets the Content-Type to the supplied string.
func SetContentType(ct string) func(next http.Handler) http.Handler {
	fn1 := func(next http.Handler) http.Handler {
		fn2 := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", ct)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn2)
	}
	return fn1
}

// GZIPContentType is a middleware that sets the Content-Type to `application/gzip`.
func GZIPContentType(next http.Handler) http.Handler {
	return SetContentType("application/gzip")(next)
}

// JSONContentType is a middleware that sets the Content-Type to `application/json`.
func JSONContentType(next http.Handler) http.Handler {
	return SetContentType("application/json")(next)
}
