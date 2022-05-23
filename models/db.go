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

func (edb *ExportDB) Create(payload *ExportPayload) error {
	return edb.DB.Create(&payload).Error
}

func (edb *ExportDB) Delete(exportUUID uuid.UUID, user User) error {
	result := edb.DB.Where(&ExportPayload{ID: exportUUID, User: user}).Delete(&ExportPayload{})
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return result.Error
}

func (edb *ExportDB) Get(exportUUID uuid.UUID, result *ExportPayload) error {
	err := (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID}).
		Take(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return err
}

func (edb *ExportDB) GetWithUser(exportUUID uuid.UUID, user User, result *ExportPayload) error {
	err := (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID, User: user}).
		Take(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return err
}

func (edb *ExportDB) APIList(user User) (result []*APIExport, err error) {
	err = (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{User: user}).
		Find(&result).Error)
	return
}

func (edb *ExportDB) List(user User) (result []*ExportPayload, err error) {
	err = (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{User: user}).
		Find(&result).Error)
	return
}

func (edb *ExportDB) Updates(m *ExportPayload, values interface{}) error {
	return edb.DB.Model(m).Updates(values).Error
}

func (edb *ExportDB) Raw(sql string, values ...interface{}) *gorm.DB {
	return edb.DB.Raw(sql, values...)
}
