package exports

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ExportPayload struct {
	ID          string     `json:"id"`
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
