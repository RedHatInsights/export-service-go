# Database Guidelines for export-service-go

## Database and ORM Stack

- **Database**: PostgreSQL (connected via `gorm.io/driver/postgres`)
- **ORM**: GORM v2 (`gorm.io/gorm`)
- **Migrations**: `golang-migrate/migrate/v4` with raw SQL files (not GORM AutoMigrate)
- **Test database**: `fergusstrange/embedded-postgres` for an ephemeral PostgreSQL instance per test suite
- **JSON columns**: `gorm.io/datatypes` for `datatypes.JSON` fields

## Schema Design

### Tables

Two tables: `export_payloads` (parent) and `sources` (child). Both use **UUID primary keys** generated in application code via `github.com/google/uuid`, not database-generated sequences.

### Model Conventions

- Primary keys use `gorm:"type:uuid;primarykey"` tag. UUIDs are assigned in `BeforeCreate` hooks, not by the database.
- Timestamps use `gorm:"autoCreateTime"` and `gorm:"autoUpdateTime"` for `CreatedAt`/`UpdatedAt`. Optional timestamps (`CompletedAt`, `Expires`) are `*time.Time` pointers.
- Enum-like fields (`Format`, `Status`) are stored as `text` in PostgreSQL and represented as typed string constants in Go (`PayloadFormat`, `PayloadStatus`, `ResourceStatus`). They use `gorm:"type:string"`.
- The `User` struct is **embedded** directly in `ExportPayload` (not a separate table). Its fields (`AccountID`, `OrganizationID`, `Username`) are flattened into the `export_payloads` table.
- The `SourceError` struct is embedded as a pointer (`*SourceError`) in `Source`, with its fields (`Code`, `Message`) stored as columns directly on the `sources` table.
- JSON data is stored using `datatypes.JSON` with `gorm:"type:json"` (mapped to `jsonb` in the migration SQL).

### Foreign Keys and Cascades

Sources reference their parent via `ExportPayloadID uuid` with `gorm:"foreignKey:ExportPayloadID"` on the parent's `Sources` field. The migration SQL defines `ON DELETE CASCADE` so deleting a payload removes its sources.

### Indexes

A composite index exists on `(account_id, organization_id, username)` on `export_payloads`, created with `CREATE INDEX CONCURRENTLY`. This supports the multi-tenant query pattern where all reads are scoped by user identity.

## Migration Practices

### File-based migrations with golang-migrate

- Migration files live in `db/migrations/` with the naming convention `NNNNNN_description.{up,down}.sql`.
- Numbering is sequential six-digit zero-padded (e.g., `000001`, `000002`).
- Migrations are **raw SQL**, not GORM AutoMigrate. Schema changes must always be a new migration file pair.
- `down` migrations revert exactly one step (`m.Steps(-1)`), while `up` applies all pending.
- The migration runner is invoked via CLI: `export-service migrate_db upgrade` or `export-service migrate_db downgrade`.
- The migration uses `database/sql` (not GORM) to connect: `db.OpenPostgresDB()` returns `*sql.DB`, separate from the GORM connection.

### Migration rules

- Use `CREATE INDEX CONCURRENTLY` and `DROP INDEX CONCURRENTLY` for index changes to avoid locking the table.
- Always provide both `.up.sql` and `.down.sql` files.
- Down migrations should be the exact inverse of up migrations.

## Database Access Layer (`models` package)

### DBInterface pattern

All database access goes through the `DBInterface` interface defined in `models/db.go`. This enables test mocking and decouples handlers from the concrete GORM implementation.

```go
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
```

The concrete implementation is `ExportDB` which holds `*gorm.DB` and `*config.ExportConfig`.

### Key rules

- **Never use `*gorm.DB` directly in handlers.** Always pass `DBInterface`.
- The `ExportDB` struct is instantiated in `main` and injected into handler structs (`Export`, `Internal`).
- Error wrapping: translate `gorm.ErrRecordNotFound` to the package-level `ErrRecordNotFound` sentinel error. Callers switch on this sentinel, not on GORM errors.

## Query Patterns

### Multi-tenancy

Every user-facing query is scoped by `User` (account_id, organization_id, username). Use GORM's struct-based `Where`:
```go
edb.DB.Where(&ExportPayload{ID: exportUUID, User: user})
```

### Eager loading

Use `Preload("Sources")` when you need the child `Sources` relation. The `Get` and `GetWithUser` methods always preload sources. The `List` and `APIList` methods do not.

### Pagination and sorting

- Pagination uses `Limit` and `Offset` passed from middleware, applied via GORM's `.Limit(limit).Offset(offset)`.
- Sorting uses `db.Order(fmt.Sprintf("%s %s", sort, dir))` where `sort` and `dir` are validated in middleware to a fixed allowlist (`name`, `created_at`, `expires` / `asc`, `desc`).
- Count is retrieved with `.Count(&count)` before applying limit/offset.

### Filtering with joins

When filtering by `application` or `resource` (fields on the `sources` table), an explicit join is added:
```go
db = db.Joins("JOIN sources ON sources.export_payload_id = export_payloads.id")
```
This join is only added when those filter parameters are present.

### Date range filtering

Date filters use `BETWEEN` with a one-day range:
```go
db.Where("export_payloads.created_at BETWEEN ? AND ?", params.Created, params.Created.AddDate(0, 0, 1))
```

### Raw SQL usage

Raw SQL is used sparingly, only when GORM's API is insufficient:
- `SetSourceStatus` uses `db.Raw("UPDATE sources SET ...")` for updating a child record with parameterized values.
- `DeleteExpiredExports` uses a raw SQL interval expression: `now() > expires + interval 'N days'`.
- Always use parameterized queries for user-supplied values to prevent SQL injection.

### GORM `clause` usage

The `DeleteExpiredExports` method uses `clause.Returning` to get deleted record details in a single query:
```go
edb.DB.Clauses(clause.Returning{Columns: columnsToReturn}).Where(...).Delete(&deletedExports)
```

## GORM Hooks

The `BeforeCreate` hook on `ExportPayload` handles:
1. Generating a UUID for the payload.
2. Setting a default expiration date if none provided.
3. Generating UUIDs for all child `Sources` and linking them to the parent via `ExportPayloadID`.

No other hooks are used. Do not add `AfterCreate`, `BeforeUpdate`, etc. without clear justification.

## Transaction Handling

This codebase does **not** use explicit GORM transactions (`db.Transaction()` or `db.Begin()/Commit()/Rollback()`). Status updates are performed as individual operations. If you need to add transactional behavior, wrap related operations in `edb.DB.Transaction(func(tx *gorm.DB) error { ... })`.

## Status Update Pattern

Status transitions on `ExportPayload` are done through dedicated methods on the model that accept `DBInterface`:
```go
func (ep *ExportPayload) SetStatusComplete(db DBInterface, t *time.Time, s3key string) error
func (ep *ExportPayload) SetStatusFailed(db DBInterface) error
func (ep *ExportPayload) SetStatusRunning(db DBInterface) error
```
These use `db.Updates(ep, values)` which calls GORM's `Model(m).Updates(values)`, updating only the specified fields. Follow this pattern for new status transitions.

## Testing Conventions

### Test database setup

- Each test suite (`models_test`, `exports_test`) starts an embedded PostgreSQL instance in `BeforeSuite` and stops it in `AfterSuite`.
- Database setup uses `utils.CreateTestDB()` which starts embedded-postgres, opens a GORM connection, and runs all migrations via `db.PerformDbMigration`.
- Tests clean up with `testGormDB.Exec("DELETE FROM export_payloads")` before each test group -- not `TRUNCATE`, and not dropping/recreating tables.

### Test framework

- Tests use **Ginkgo v2** with **Gomega** matchers.
- The `purge_expired_exports_test.go` file uses a `//go:build sql` build tag for tests that require a real (non-embedded) database connection.

### Test data manipulation

Tests modify data directly via `testGormDB.Exec()` for raw SQL or `testGormDB.Save()` for GORM updates. This is acceptable in tests but not in production code (which should go through `DBInterface`).

## Adding New Database Features Checklist

1. Define the schema change as a new migration pair in `db/migrations/`.
2. Update the GORM model structs in `models/models.go`.
3. Add any new query methods to `DBInterface` and implement on `ExportDB`.
4. Use parameterized queries for all user input.
5. Scope all user-facing queries by `User` fields.
6. Add tests using the embedded PostgreSQL pattern from existing test suites.
