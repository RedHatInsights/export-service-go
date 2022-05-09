/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package errors

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Msg  interface{} `json:"message"`
	Code int         `json:"code"`
}

// JSONError writes the supplied error and status code to the ResponseWriter
func JSONError(w http.ResponseWriter, err interface{}, code int) {
	e := Error{Msg: err, Code: code}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(e)
}

// BadRequestError returns a 400 json response
func BadRequestError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusBadRequest)
}

// InternalServerError returns a 500 json response
func InternalServerError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusInternalServerError)
}

// NotFoundError returns a 404 json response
func NotFoundError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusNotFound)
}

// NotImplementedError returns a 501 json response
func NotImplementedError(w http.ResponseWriter) {
	JSONError(w, "not implemented", http.StatusNotImplemented)
}
