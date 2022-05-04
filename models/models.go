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
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Expires     *time.Time     `json:"expires,omitempty"`
	RequestID   string         `json:"request_id"`
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
		return fmt.Errorf("failed to get sources: %v", err)
	}
	for _, source := range sources {
		if source.ID == uid {
			source.Status = status
			source.StatusMsg = msg
			break
		}
	}
	if err := ep.SetSources(sources); err != nil {
		return fmt.Errorf("failed to set sources for export_payload: %v", err)
	}
	return nil
}

func (ep *ExportPayload) GetAllSourcesFinished() (bool, error) {
	sources, err := ep.GetSources()
	if err != nil {
		return false, fmt.Errorf("failed to get sources: %v", err)
	}
	for _, source := range sources {
		if source.Status == RPending {
			return false, nil
		}
	}
	return true, nil
}

func (ep *ExportPayload) convertToAPI() (*APIExportWithSources, error) {
	sources, err := ep.GetSources()
	if err != nil {
		return nil, err
	}
	apiSources := []APISources{}
	for _, source := range sources {
		s := APISources{
			ID:       source.ID,
			Resource: source.Resource,
			Status:   string(source.Status),
		}
		apiSources = append(apiSources, s)
	}
	result := &APIExportWithSources{
		ID:        ep.ID,
		Name:      ep.Name,
		CreatedAt: ep.CreatedAt,
		Format:    string(ep.Format),
		Status:    string(ep.Status),
		Sources:   apiSources,
	}
	return result, nil
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

type APIExportWithSources struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	CreatedAt   time.Time    `json:"created"`
	CompletedAt *time.Time   `json:"completed,omitempty"`
	Expires     *time.Time   `json:"expires,omitempty"`
	Format      string       `json:"format"`
	Status      string       `json:"status"`
	Sources     []APISources `json:"sources"`
}

type APISources struct {
	ID       uuid.UUID `json:"id"`
	Resource string    `json:"resource"`
	Status   string    `json:"status"`
}

type ExportDB struct {
	DB *gorm.DB
}

type DBInterface interface {
	APICreate(payload *ExportPayload) (*APIExportWithSources, error)
	APIGet(exportUUID uuid.UUID) (*APIExportWithSources, error)
	APIGetWithUser(exportUUID uuid.UUID, user User) (*APIExportWithSources, error)
	APIList(user User) (result []*APIExport, err error)

	Create(payload *ExportPayload) (int64, error)
	Delete(exportUUID uuid.UUID, user User) (int64, error)
	Get(exportUUID uuid.UUID) (*ExportPayload, error)
	GetWithUser(exportUUID uuid.UUID, user User) (*ExportPayload, error)
	List(user User) (result []*ExportPayload, err error)
	Save(m *ExportPayload) (int64, error)
}

func (em *ExportDB) APICreate(payload *ExportPayload) (*APIExportWithSources, error) {
	_, err := em.Create(payload)
	if err != nil {
		return nil, err
	}
	return payload.convertToAPI()
}

func (em *ExportDB) Create(payload *ExportPayload) (int64, error) {
	result := em.DB.Create(&payload)
	return result.RowsAffected, result.Error
}

func (em *ExportDB) Delete(exportUUID uuid.UUID, user User) (int64, error) {
	result := (em.DB.Where(&ExportPayload{
		ID: exportUUID, User: user}).
		Delete(&ExportPayload{}))
	return result.RowsAffected, result.Error
}

func (em *ExportDB) APIGet(exportUUID uuid.UUID) (*APIExportWithSources, error) {
	payload, err := em.Get(exportUUID)
	if err != nil {
		return nil, err
	}
	return payload.convertToAPI()
}

func (em *ExportDB) Get(exportUUID uuid.UUID) (*ExportPayload, error) {
	result := &ExportPayload{}
	err := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID}).
		Find(&result).Error)
	return result, err
}

func (em *ExportDB) APIGetWithUser(exportUUID uuid.UUID, user User) (*APIExportWithSources, error) {
	payload, err := em.GetWithUser(exportUUID, user)
	if err != nil {
		return nil, err
	}
	return payload.convertToAPI()
}

func (em *ExportDB) GetWithUser(exportUUID uuid.UUID, user User) (*ExportPayload, error) {
	result := &ExportPayload{}
	err := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID, User: user}).
		Find(&result).Error)
	return result, err
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
