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
	ID        uuid.UUID      `json:"id"`
	Status    ResourceStatus `json:"status"`
	StatusMsg *string        `json:"status_msg,omitempty"`
	Resource  string         `json:"resource"`
	Filters   datatypes.JSON `json:"filters"`
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
	err = ep.SetSources(sources)
	return err
}

func (ep *ExportPayload) SetStatusComplete(t *time.Time, s3key string) {
	ep.Status = Complete
	ep.CompletedAt = t
	ep.S3Key = s3key
}

func (ep *ExportPayload) SetStatusFailed()  { ep.Status = Failed }
func (ep *ExportPayload) SetStatusRunning() { ep.Status = Running }

func (ep *ExportPayload) GetSources() ([]*Source, error) {
	var sources []*Source
	err := json.Unmarshal(ep.Sources, &sources)
	return sources, err
}

func (ep *ExportPayload) SetSources(sources []*Source) error {
	out, err := json.Marshal(sources)
	ep.Sources = out
	return err
}

func (ep *ExportPayload) SetSourceStatus(uid uuid.UUID, status ResourceStatus, msg *string) error {
	sources, err := ep.GetSources()
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}
	for _, source := range sources {
		if source.ID == uid {
			source.Status = status
			source.StatusMsg = msg
			break
		}
	}
	if err := ep.SetSources(sources); err != nil {
		return fmt.Errorf("failed to set sources for export_payload: %w", err)
	}
	return nil
}

func (ep *ExportPayload) GetAllSourcesFinished() (bool, error) {
	sources, err := ep.GetSources()
	if err != nil {
		return false, fmt.Errorf("failed to get sources: %w", err)
	}
	for _, source := range sources {
		if source.Status == RPending {
			return false, nil
		}
	}
	return true, nil
}

// APIExport represents select fields of the ExportPayload which are returned to the user
type APIExport struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created"`
	CompletedAt *time.Time `json:"completed,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
}

type ExportDB struct {
	DB *gorm.DB
}

type DBInterface interface {
	APIList(user User) (result []*APIExport, err error)

	Create(payload *ExportPayload) (int64, error)
	Delete(exportUUID uuid.UUID, user User) (int64, error)
	Get(exportUUID uuid.UUID, result *ExportPayload) (int64, error)
	GetWithUser(exportUUID uuid.UUID, user User, result *ExportPayload) (int64, error)
	List(user User) (result []*ExportPayload, err error)
	Save(m *ExportPayload) (int64, error)
}

func (em *ExportDB) Create(payload *ExportPayload) (int64, error) {
	result := em.DB.Create(&payload)
	return result.RowsAffected, result.Error
}

func (em *ExportDB) Delete(exportUUID uuid.UUID, user User) (int64, error) {
	result := (em.DB.Where(&ExportPayload{
		ID: exportUUID, User: user,
	}).
		Delete(&ExportPayload{}))
	return result.RowsAffected, result.Error
}

func (em *ExportDB) Get(exportUUID uuid.UUID, result *ExportPayload) (int64, error) {
	query := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID}).
		Find(&result))
	return query.RowsAffected, query.Error
}

func (em *ExportDB) GetWithUser(exportUUID uuid.UUID, user User, result *ExportPayload) (int64, error) {
	query := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID, User: user}).
		Find(&result))
	return query.RowsAffected, query.Error
}

func (em *ExportDB) APIList(user User) (result []*APIExport, err error) {
	err = (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{User: user}).
		Find(&result).Error)
	return
}

func (em *ExportDB) List(user User) (result []*ExportPayload, err error) {
	err = (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{User: user}).
		Find(&result).Error)
	return
}

func (em *ExportDB) Save(m *ExportPayload) (int64, error) {
	result := em.DB.Save(m)
	return result.RowsAffected, result.Error
}
