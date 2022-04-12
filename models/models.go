package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ExportPayload struct {
	ID             string         `gorm:"type:uuid;primarykey" json:"id"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	Expires        *time.Time     `json:"expires,omitempty"`
	Name           string         `json:"name"`
	Application    string         `json:"application"`
	Format         string         `json:"format"`
	Resources      datatypes.JSON `json:"resources"`
	AccountID      string         `json:"account_id"`
	OrganizationID string         `json:"organization_id"`
	Username       string         `json:"username"`
}

func (ep *ExportPayload) BeforeCreate(tx *gorm.DB) error {
	ep.ID = uuid.NewString()
	return nil
}
