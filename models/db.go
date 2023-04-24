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
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Expires     *time.Time `json:"expires_at,omitempty"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
}

type ExportDB struct {
	DB  *gorm.DB
	Cfg *config.ExportConfig
}

type DBInterface interface {
	APIList(user User, params *QueryParams, offset, limit int, sort, dir string) (result []*APIExport, count int64, err error)

	Create(payload *ExportPayload) (result *ExportPayload, err error)
	Delete(exportUUID uuid.UUID, user User) error
	Get(exportUUID uuid.UUID) (result *ExportPayload, err error)
	GetWithUser(exportUUID uuid.UUID, user User) (result *ExportPayload, err error)
	List(user User) (result []*ExportPayload, err error)
	Raw(sql string, values ...interface{}) *gorm.DB
	Updates(m *ExportPayload, values interface{}) error
	DeleteExpiredExports() error
}

var ErrRecordNotFound = errors.New("record not found")

func (edb *ExportDB) Create(payload *ExportPayload) (*ExportPayload, error) {
	result := edb.DB.Create(&payload)
	return payload, result.Error
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
		Preload("Sources").
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
		Preload("Sources").
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

	// filtering by application and resource, joining the export_payloads table to the sources table
	if params.Application != "" || params.Resource != "" {
		db = db.Joins("JOIN sources ON sources.export_payload_id = export_payloads.id")

		if params.Application != "" {
			db = db.Where("sources.application = ?", params.Application)
		}

		if params.Resource != "" {
			db = db.Where("sources.resource = ?", params.Resource)
		}
	}

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
	log := logger.Get()

	columnsToReturn := []clause.Column{{Name: "id"}, {Name: "account_id"}, {Name: "organization_id"}, {Name: "username"}}
	expiredExportsClause := fmt.Sprintf("now() > expires + interval '%d days'", edb.Cfg.ExportExpiryDays)

	var deletedExports []ExportPayload
	err := edb.DB.Clauses(clause.Returning{Columns: columnsToReturn}).Where(expiredExportsClause).Delete(&deletedExports).Error
	if err != nil {
		log.Error("Unable to remove expired exports from the database", "error", err)
		return err
	}

	for _, export := range deletedExports {
		log.Debugw("Deleted expired export",
			"id", export.ID,
			"org_id", export.OrganizationID,
			"account", export.AccountID,
			"username", export.Username)
	}

	return nil
}
