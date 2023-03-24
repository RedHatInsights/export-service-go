/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware

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
