/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package models

import (
	"encoding/json"
	"fmt"
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
	RSuccess ResourceStatus = "success"
	RFailed  ResourceStatus = "failed"
)

// URLParams represent the `exportUUID`, `resourceUUID`, and `application` found in
// the url. These are added to the request context using the URLParams middleware.
type URLParams struct {
	ExportUUID   uuid.UUID
	Application  string
	ResourceUUID uuid.UUID
}

type ExportPayload struct {
	ID             uuid.UUID      `gorm:"type:uuid;primarykey" json:"id"`
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
	ID       uuid.UUID      `json:"id"`
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
	ep.ID = uuid.New()
	ep.Status = Pending
	sources, err := ep.GetSources()
	if err != nil {
		return err
	}
	for _, source := range sources {
		source.ID = uuid.New()
		source.Status = RPending
	}
	err = ep.SaveSources(sources)
	return err
}

func (ep *ExportPayload) GetSources() (sources []*Source, err error) {
	err = json.Unmarshal(ep.Sources, &sources)
	return
}

func (ep *ExportPayload) GetSource(uid uuid.UUID) (*Source, error) {
	var sources []*Source
	if err := json.Unmarshal(ep.Sources, &sources); err != nil {
		return nil, err
	}
	for _, source := range sources {
		if source.ID == uid {
			return source, nil
		}
	}
	return nil, fmt.Errorf("source uuid not found")
}

func (ep *ExportPayload) SaveSources(sources []*Source) (err error) {
	out, err := json.Marshal(sources)
	ep.Sources = out
	return
}
