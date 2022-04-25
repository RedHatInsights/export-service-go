/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type PayloadFormat string

const (
	CSV  PayloadFormat = "csv"
	JSON PayloadFormat = "json"
)

type PayloadStatus string

const (
	Pending  PayloadStatus = "pending"
	Running  PayloadStatus = "running"
	Complete PayloadStatus = "complete"
)

type ResourceStatus string

const (
	RPending ResourceStatus = "pending"
	RSuccess ResourceStatus = "success"
	RFailure ResourceStatus = "failure"
)

// URLParams represent the `exportUUID`, `resourceUUID`, and `application` found in
// the url. These are added to the request context using the URLParams middleware.
type URLParams struct {
	ExportUUID   uuid.UUID
	Application  string
	ResourceUUID uuid.UUID
}

type ExportPayload struct {
	ID             uuid.UUID     `gorm:"type:uuid;primarykey" json:"id"`
	CreatedAt      time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	Expires        *time.Time    `json:"expires,omitempty"`
	RequestID      string        `json:"request_id"`
	Name           string        `json:"name"`
	Application    string        `json:"application"`
	Format         PayloadFormat `gorm:"type:string" json:"format"`
	Status         PayloadStatus `gorm:"type:string" json:"status"`
	Sources        []*Source     `gorm:"type:json" json:"sources"`
	S3DownloadLink string        `json:"-"`
	User
}

type Source struct {
	ID       uuid.UUID      `json:"id"`
	Resource string         `json:"resource"`
	Filters  datatypes.JSON `json:"filters"`
}

type User struct {
	AccountID      string `json:"-"`
	OrganizationID string `json:"-"`
	Username       string `json:"-"`
}
