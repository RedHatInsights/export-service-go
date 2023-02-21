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

func (pf *PayloadFormat) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "csv":
		*pf = CSV
	case "json":
		*pf = JSON
	default:
		return fmt.Errorf("unknown payload format: %s", s)
	}
	return nil
}

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

// QueryParams for the /export/v1/exports endpoint
type QueryParams struct {
	Name        string
	Created     time.Time
	Expires     time.Time
	Status      string
	Application string
	Resource    string
}

// TODO: Seperate database struct and request struct
type ExportPayload struct {
	ID          uuid.UUID `gorm:"type:uuid;primarykey"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
	CompletedAt *time.Time
	Expires     *time.Time
	RequestID   string
	Name        string
	Format      PayloadFormat `gorm:"type:string"`
	Status      PayloadStatus `gorm:"type:string"`
	Sources     []Source      `gorm:"foreignKey:ExportPayloadID"`
	S3Key       string
	User
}

type Source struct {
	ID              uuid.UUID `gorm:"type:uuid;primarykey"`
	ExportPayloadID uuid.UUID `gorm:"type:uuid"`
	Application     string
	Status          ResourceStatus
	Resource        string
	Filters         datatypes.JSON `gorm:"type:json"`
	*SourceError
}

type SourceError struct {
	Message string
	Code    int
}

type User struct {
	AccountID      string
	OrganizationID string
	Username       string
}

func (ep *ExportPayload) GetSource(db DBInterface, uid uuid.UUID) (int, *Source, error) {
	sources, err := ep.GetSources(db)
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

func (ep *ExportPayload) GetSources(db DBInterface) ([]*Source, error) { // TODO: Remove usage of pointers here
	var sources []*Source
	//err := json.Unmarshal(ep.Sources, &sources)

	sql := db.Raw("SELECT * FROM export_sources WHERE export_sources.export_payload_id = ?", ep.ID)
	err := sql.Find(&sources).Error

	fmt.Println("GET SOURCES123: ", sources, err)

	return sources, err
}

func (es *Source) GetFilters() (map[string]string, error) {
	var filters map[string]string
	err := json.Unmarshal([]byte(es.Filters), &filters)
	return filters, err
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
	_, _, err := ep.GetSource(db, uid)
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	var sql *gorm.DB
	if sourceError == nil {
		// set the status and remove 'code' and 'message' fields if they exist
		//sqlStr := fmt.Sprintf("UPDATE export_payloads SET sources = jsonb_set(sources, '{%d,status}', '\"%s\"', false) #- '{%d,code}' #- '{%d,message}' WHERE id='%s'", idx, status, idx, idx, ep.ID)
		sqlStr := fmt.Sprintf("UPDATE export_sources SET status = '%s' WHERE id='%s'", status, uid)
		sql = db.Raw(sqlStr)
	} else {
		// set status and add 'code' and 'message' fields
		// the `code` and `message` are user inputs, so they are parameterized to prevent sql injection
		//sqlStr := fmt.Sprintf("UPDATE export_payloads SET sources = jsonb_set(sources, '{%d}', sources->%d || jsonb_build_object('status', '%s', 'code', ?::int, 'message', ?::text), true) WHERE id='%s'", idx, idx, status, ep.ID)
		// TODO: Do we need to parameterize the code and message to prevent sql injection here as well?
		sqlStr := fmt.Sprintf("UPDATE export_sources SET status = '%s', code = %d, message = '%s' WHERE id='%s'", status, sourceError.Code, sourceError.Message, uid)
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
//   - StatusError - failed to retrieve sources
//   - StatusComplete - sources are all complete as success
//   - StatusPending - sources are still pending
//   - StatusPartial - sources are all complete, some sources are a failure
//   - StatusFailed - all sources have failed
func (ep *ExportPayload) GetAllSourcesStatus(db DBInterface) (int, error) {
	sources, err := ep.GetSources(db)
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
