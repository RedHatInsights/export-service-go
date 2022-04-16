/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package exports

import (
	"net/http"
	"time"
)

// APIExport represents select fields of the ExportPayload which are returned to the user
type APIExport struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created"`
	CompletedAt *time.Time `json:"completed,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
}

func (e *APIExport) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

type urlParams struct {
	exportUUID   string
	application  string
	resourceUUID string
}
