package models

import (
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

type ExportPayload struct {
	ID             string        `gorm:"type:uuid;primarykey" json:"id"`
	CreatedAt      time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	Expires        *time.Time    `json:"expires,omitempty"`
	Name           string        `json:"name"`
	Application    string        `json:"application"`
	Format         PayloadFormat `gorm:"type:string" json:"format"`
	Status         PayloadStatus `gorm:"type:string" json:"status"`
	Sources        []*Source     `gorm:"type:json" json:"sources"`
	S3DownloadLink string        `json:"-"`
	User
}

type Source struct {
	ID       string         `json:"id"`
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
	for _, source := range ep.Sources {
		source.ID = uuid.NewString()
	}
	return nil
}
