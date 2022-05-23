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
	Partial  PayloadStatus = "partial"
	Pending  PayloadStatus = "pending"
	Running  PayloadStatus = "running"
	Complete PayloadStatus = "complete"
	Failed   PayloadStatus = "failed"
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
	ID          uuid.UUID      `gorm:"type:uuid;primarykey" json:"id"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"-"`
	CompletedAt *time.Time     `json:"completed,omitempty"`
	Expires     *time.Time     `json:"expires,omitempty"`
	RequestID   string         `json:"-"`
	Name        string         `json:"name"`
	Application string         `json:"application"`
	Format      PayloadFormat  `gorm:"type:string" json:"format"`
	Status      PayloadStatus  `gorm:"type:string" json:"status"`
	Sources     datatypes.JSON `gorm:"type:json" json:"sources"`
	S3Key       string         `json:"-"`
	User
}

type Source struct {
	ID       uuid.UUID      `json:"id"`
	Status   ResourceStatus `json:"status"`
	Resource string         `json:"resource"`
	Filters  datatypes.JSON `json:"filters"`
	*SourceError
}

type SourceError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
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
	out, err := json.Marshal(sources)
	ep.Sources = out
	return err
}

func (ep *ExportPayload) GetSource(uid uuid.UUID) (int, *Source, error) {
	sources, err := ep.GetSources()
	if err != nil {
		return -1, nil, fmt.Errorf("failed to get sources: %w", err)
	}
	for idx, source := range sources {
		if source.ID == uid {
			return idx, source, nil
		}
	}
	return -1, nil, fmt.Errorf("source `%s` not found", uid)
}

func (ep *ExportPayload) GetSources() ([]*Source, error) {
	var sources []*Source
	err := json.Unmarshal(ep.Sources, &sources)
	return sources, err
}

func (ep *ExportPayload) SetStatusComplete(db DBInterface, t *time.Time, s3key string) error {
	values := ExportPayload{
		Status:      Complete,
		CompletedAt: t,
		S3Key:       s3key,
	}
	return db.Updates(ep, values)
}

func (ep *ExportPayload) SetStatusPartial(db DBInterface, t *time.Time, s3key string) error {
	values := ExportPayload{
		Status:      Partial,
		CompletedAt: t,
		S3Key:       s3key,
	}
	return db.Updates(ep, values)
}

func (ep *ExportPayload) SetStatusFailed(db DBInterface) error {
	t := time.Now()
	values := ExportPayload{
		Status:      Failed,
		CompletedAt: &t,
	}
	return db.Updates(ep, values)
}

func (ep *ExportPayload) SetStatusRunning(db DBInterface) error {
	values := ExportPayload{Status: Running}
	return db.Updates(ep, values)
}

func (ep *ExportPayload) SetSourceStatus(db DBInterface, uid uuid.UUID, status ResourceStatus, sourceError *SourceError) error {
	idx, _, err := ep.GetSource(uid)
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	var sql *gorm.DB
	if sourceError == nil {
		// set the status and remove 'code' and 'message' fields if they exist
		sqlStr := fmt.Sprintf("UPDATE export_payloads SET sources = jsonb_set(sources, '{%d,status}', '\"%s\"', false) #- '{%d,code}' #- '{%d,message}' WHERE id='%s'", idx, status, idx, idx, ep.ID)
		sql = db.Raw(sqlStr)
	} else {
		// set status and add 'code' and 'message' fields
		// the `code` and `message` are user inpurts, so they are parameterized to prevent sql injection
		sqlStr := fmt.Sprintf("UPDATE export_payloads SET sources = jsonb_set(sources, '{%d}', sources->%d || jsonb_build_object('status', '%s', 'code', ?::int, 'message', ?::text), true) WHERE id='%s'", idx, idx, status, ep.ID)
		sql = db.Raw(sqlStr, sourceError.Code, sourceError.Message)
	}
	return sql.Scan(&ep).Error
}

const (
	StatusError = iota - 1
	StatusFailed
	StatusPending
	StatusPartial
	StatusComplete
)

// GetAllSourcesStatus gets the status for all of the sources. This function can return these different states:
//   *  StatusError - failed to retrieve sources
//   *  StatusComplete - sources are all complete as success
//   *  StatusPending - sources are still pending
//   *  StatusPartial - sources are all complete, some sources are a failure
//   *  StatusFailed - all sources have failed
func (ep *ExportPayload) GetAllSourcesStatus() (int, error) {
	sources, err := ep.GetSources()
	if err != nil {
		// we do not know the status of the sources. as far as we know, there is nothing to zip.
		return StatusError, err
	}
	failedCount := 0
	for _, source := range sources {
		if source.Status == RPending {
			// there are more sources in a pending state. there is nothing to zip yet.
			return StatusPending, nil
		} else if source.Status == RFailed {
			failedCount += 1
		}
	}
	if failedCount == len(sources) {
		// return 2 as there is nothing to zip into a payload.
		return StatusFailed, nil
	}
	if failedCount > 0 {
		return StatusPartial, nil
	}

	// return 0 as there is at least 1 source that needs to be zipped.
	return StatusComplete, nil
}
