/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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
	RRunning ResourceStatus = "running"
	RSuccess ResourceStatus = "success"
	RFailed  ResourceStatus = "failed"
)

// URLParams represent the `exportUUID`, `resourceUUID`, and `application` found in
// the url. These are added to the request context using the URLParams middleware.
type URLParams struct {
	ExportUUID   string
	Application  string
	ResourceUUID string
}

type ExportPayload struct {
	ID             string         `gorm:"type:uuid;primarykey" json:"id"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	Expires        *time.Time     `json:"expires,omitempty"`
	RequestID      string         `json:"request_id"`
	Name           string         `json:"name"`
	Application    string         `json:"application"`
	Format         PayloadFormat  `gorm:"type:string" json:"format"`
	Status         PayloadStatus  `gorm:"type:string" json:"status"`
	Sources        datatypes.JSON `gorm:"type:json" json:"sources"`
	S3DownloadLink string         `json:"-"`
	User
}

type Source struct {
	ID       string         `json:"id"`
	Status   ResourceStatus `json:"status"`
	Resource string         `json:"resource"`
	Filters  datatypes.JSON `json:"filters"`
}

type User struct {
	AccountID      string `json:"-"`
	OrganizationID string `json:"-"`
	Username       string `json:"-"`
}

func (ep *ExportPayload) BeforeCreate(tx *gorm.DB) error {
	ep.ID = uuid.NewString()
	ep.Status = Pending
	sources, err := ep.GetSources()
	if err != nil {
		return err
	}
	for _, source := range sources {
		source.ID = uuid.NewString()
		source.Status = RPending
	}
	err = ep.SaveSources(sources)
	return err
}

func (ep *ExportPayload) GetSources() (sources []*Source, err error) {
	err = json.Unmarshal(ep.Sources, &sources)
	return
}

func (ep *ExportPayload) SaveSources(sources []*Source) (err error) {
	out, err := json.Marshal(sources)
	ep.Sources = out
	return
}
