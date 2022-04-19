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

func JSONError(w http.ResponseWriter, err interface{}, code int) {
	e := Error{Msg: err, Code: code}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(e)
}

func BadRequestError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusBadRequest)
}
func InternalServerError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusInternalServerError)
}
func NotFoundError(w http.ResponseWriter, err interface{}) {
	JSONError(w, err, http.StatusNotFound)
}
func NotImplementedError(w http.ResponseWriter) {
	JSONError(w, "not implemented", http.StatusNotImplemented)
}
