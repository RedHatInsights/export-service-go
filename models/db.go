package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

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

	Create(payload *ExportPayload) error
	Delete(exportUUID uuid.UUID, user User) error
	Get(exportUUID uuid.UUID, result *ExportPayload) error
	GetWithUser(exportUUID uuid.UUID, user User, result *ExportPayload) error
	List(user User) (result []*ExportPayload, err error)
	Raw(sql string, values ...interface{}) *gorm.DB
	Updates(m *ExportPayload, values interface{}) error
}

var ErrRecordNotFound = errors.New("record not found")

func (em *ExportDB) Create(payload *ExportPayload) error {
	return em.DB.Create(&payload).Error
}

func (em *ExportDB) Delete(exportUUID uuid.UUID, user User) error {
	err := (em.DB.Where(&ExportPayload{ID: exportUUID, User: user}).Delete(&ExportPayload{})).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return err
}

func (em *ExportDB) Get(exportUUID uuid.UUID, result *ExportPayload) error {
	err := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID}).
		Find(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return err
}

func (em *ExportDB) GetWithUser(exportUUID uuid.UUID, user User, result *ExportPayload) error {
	err := (em.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID, User: user}).
		Find(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return err
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

func (em *ExportDB) Updates(m *ExportPayload, values interface{}) error {
	return em.DB.Model(m).Updates(values).Error
}

func (em *ExportDB) Raw(sql string, values ...interface{}) *gorm.DB {
	return em.DB.Raw(sql, values...)
}
