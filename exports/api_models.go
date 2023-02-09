package exports

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// type PayloadStatus string

// const (
// 	Partial  PayloadStatus = "partial"
// 	Pending  PayloadStatus = "pending"
// 	Running  PayloadStatus = "running"
// 	Complete PayloadStatus = "complete"
// 	Failed   PayloadStatus = "failed"
// )

// type PayloadFormat string

// const (
// 	CSV  PayloadFormat = "csv"
// 	JSON PayloadFormat = "json"
// )

// type ResourceStatus string

// const (
// 	RPending ResourceStatus = "pending"
// 	RSuccess ResourceStatus = "success"
// 	RFailed  ResourceStatus = "failed"
// )

type ExportPayload struct {
	ID          uuid.UUID  `json:"id"`
	CreatedAt   time.Time  `json:"created"`
	CompletedAt *time.Time `json:"completed,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Name        string     `json:"name"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
	Sources     []Source   `json:"sources"`
}

type Source struct {
	ID          uuid.UUID      `json:"id"`
	Application string         `json:"application"`
	Status      string         `json:"status"`
	Resource    string         `json:"resource"`
	Filters     datatypes.JSON `json:"filters"`
	Message     *string        `json:"message,omitempty"`
	Code        *int           `json:"code,omitempty"`
}
