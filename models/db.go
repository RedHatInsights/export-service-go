package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	DB  *gorm.DB
	Cfg *config.ExportConfig
}

type DBInterface interface {
	APIList(user User, params *QueryParams, offset, limit int, sort, dir string) (result []*APIExport, count int64, err error)

	Create(payload *ExportPayload) error
	Delete(exportUUID uuid.UUID, user User) error
	Get(exportUUID uuid.UUID) (result *ExportPayload, err error)
	GetWithUser(exportUUID uuid.UUID, user User) (result *ExportPayload, err error)
	List(user User) (result []*ExportPayload, err error)
	Raw(sql string, values ...interface{}) *gorm.DB
	Updates(m *ExportPayload, values interface{}) error
	DeleteExpiredExports() error
}

var ErrRecordNotFound = errors.New("record not found")

func (edb *ExportDB) Create(payload *ExportPayload) error {
	if payload.Expires == nil {
		expirationTime := time.Now().AddDate(0, 0, config.ExportCfg.ExportExpiryDays)
		payload.Expires = &expirationTime
	}
	return edb.DB.Create(&payload).Error
}

func (edb *ExportDB) Delete(exportUUID uuid.UUID, user User) error {
	result := edb.DB.Where(&ExportPayload{ID: exportUUID, User: user}).Delete(&ExportPayload{})
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return result.Error
}

func (edb *ExportDB) Get(exportUUID uuid.UUID) (result *ExportPayload, err error) {
	err = (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID}).
		Take(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, ErrRecordNotFound
	}
	return
}

func (edb *ExportDB) GetWithUser(exportUUID uuid.UUID, user User) (result *ExportPayload, err error) {
	err = (edb.DB.Model(&ExportPayload{}).
		Where(&ExportPayload{ID: exportUUID, User: user}).
		Take(&result)).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, ErrRecordNotFound
	}
	return
}

func (edb *ExportDB) APIList(user User, params *QueryParams, offset, limit int, sort, dir string) (result []*APIExport, count int64, err error) {
	db := edb.DB.Model(&ExportPayload{}).Where(&ExportPayload{User: user})

	// filter by name, export status, created, expires, application, resource
	if params.Name != "" {
		db = db.Where("export_payloads.name = ?", params.Name)
	}

	if params.Status != "" {
		db = db.Where("export_payloads.status = ?", params.Status)
	}

	if !params.Created.IsZero() {
		db = db.Where("export_payloads.created_at BETWEEN ? AND ?", params.Created, params.Created.AddDate(0, 0, 1))
	}

	if !params.Expires.IsZero() {
		db = db.Where("export_payloads.expires BETWEEN ? AND ?", params.Expires, params.Expires.AddDate(0, 0, 1))
	}

	// TODO: filtering by application and resource needs to be implemented with a sources table
	// Currently, the sources is stored as json in the table, which is not efficient to parse

	// count total records
	db.Count(&count)

	// order by sort and dir params
	db = db.Order(fmt.Sprintf("%s %s", sort, dir)).Limit(limit).Offset(offset)

	err = db.Find(&result).Error

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

func (edb *ExportDB) DeleteExpiredExports() error {

	columnsToReturn := []clause.Column{{Name: "id"}, {Name: "account_id"}, {Name: "organization_id"}, {Name: "username"}}
	expiredExportsClause := fmt.Sprintf("now() > expires + interval '%d days'", config.ExportCfg.ExportExpiryDays)

	var deletedExports []ExportPayload
	err := edb.DB.Clauses(clause.Returning{Columns: columnsToReturn}).Where(expiredExportsClause).Delete(&deletedExports).Error
	if err != nil {
		logger.Log.Error("Unable to remove expired exports from the database", "error", err)
		return err
	}

	for _, export := range deletedExports {
		logger.Log.Debugw("Deleted expired export",
			"id", export.ID,
			"org_id", export.OrganizationID,
			"account", export.AccountID,
			"username", export.Username)
	}

	return nil
}
