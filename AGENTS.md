# AGENTS.md

This file provides onboarding guidance for AI agents working in the export-service-go repository. It covers cross-cutting conventions that span multiple domains and serves as an index to the detailed guideline files. For project description and getting-started instructions, see `README.md`.

## Guideline Docs Index

| File | Description |
|------|-------------|
| `docs/security-guidelines.md` | Dual-server auth model, PSK/identity validation, input sanitization, SQL injection prevention, S3 tenant isolation, secrets management |
| `docs/performance-guidelines.md` | Rate limiting, HTTP timeouts, payload size limits, S3 buffer sizing, Kafka producer concurrency, non-blocking response pattern, Prometheus metrics conventions |
| `docs/error-handling-guidelines.md` | Zap logging levels, HTTP error response helpers, sentinel errors, error wrapping rules, goroutine error handling, the `Logerr` helper |
| `docs/api-contracts-guidelines.md` | OpenAPI specs, request/response formats, pagination, query filtering, status enums, Kafka message contract, API model layers |
| `docs/database-guidelines.md` | PostgreSQL/GORM stack, schema design, migration practices, DBInterface pattern, query patterns, multi-tenancy scoping, GORM hooks |
| `docs/testing-guidelines.md` | Ginkgo v2 + Gomega framework, embedded Postgres, hand-written mocks, HTTP handler testing, DescribeTable for parameterized tests, build tags |
| `docs/integration-guidelines.md` | Clowder platform integration, Kafka producer setup, S3/MinIO usage, CloudWatch logging, dependency injection pattern, local dev with docker-compose |

There is also `docs/integration.md` which covers how external applications integrate with the export service (aimed at service consumers, not contributors).

## Repository Structure

```
cmd/export-service/     # Cobra CLI entrypoint: api_server, expired_export_cleaner, migrate_db subcommands
config/                 # Singleton config via viper + Clowder (config.Get())
db/                     # Database connection and golang-migrate runner
  migrations/           # Sequential SQL migration pairs (NNNNNN_description.{up,down}.sql)
exports/                # Public and internal HTTP handlers, API models, JSON error helpers
kafka/                  # Kafka producer, message types, CloudEvents schema
logger/                 # Zap SugaredLogger singleton and structured field helpers
metrics/                # Prometheus HTTP request metrics and middleware
middleware/             # Auth (identity, PSK), pagination, URL params, content-type middleware
models/                 # GORM models, DBInterface, database access layer
s3/                     # S3 client, StorageHandler interface, compress/download/upload, mock
static/spec/            # OpenAPI specs (YAML and JSON) for public and private APIs
utils/                  # Test DB fixture helper (CreateTestDB)
deploy/                 # ClowdApp manifest
```

## Cross-Cutting Conventions

### Go Module and Versioning

- Module path: `github.com/redhatinsights/export-service-go`
- Go version: 1.24+ (as specified in `go.mod` and CI workflows)
- License: Apache-2.0 with copyright headers on source files

### Package Naming

- All packages use short, lowercase, single-word names: `config`, `exports`, `kafka`, `logger`, `metrics`, `middleware`, `models`, `s3`, `utils`.
- The `cmd/export-service/` package is `package main`. All business logic lives outside `cmd/`.
- Import aliases are used when package names conflict or need clarity: `es3` for the `s3` package, `export_logger` for the `logger` package, `chi` for `github.com/go-chi/chi/v5`.

### File Naming

- Go files use `snake_case.go` (e.g., `api_server.go`, `jsonerror_response.go`, `api_models.go`).
- Test files follow `*_test.go` convention with external test packages (`package <pkg>_test`).
- Each test package has a `*_suite_test.go` file bootstrapping Ginkgo.
- Migration files: `NNNNNN_description.{up,down}.sql` with six-digit zero-padded numbers.

### Function and Variable Naming

- Exported types use PascalCase: `ExportPayload`, `StorageHandler`, `DBInterface`.
- Handler structs are named for their API scope: `Export` (public), `Internal` (private).
- Router methods follow the pattern `<Struct>Router` (e.g., `ExportRouter`, `InternalRouter`).
- Handler methods are named `<HTTPVerb><Resource>` (e.g., `PostExport`, `GetExportStatus`, `PostUpload`, `PostError`, `DeleteExport`, `ListExports`).
- Conversion functions: `DBExportToAPI()`, `APIExportToDBExport()`, `mapUsertoModelUser()`.
- Status constants use bare names for payload (`Pending`, `Running`, `Complete`, `Failed`, `Partial`) and `R`-prefixed names for resource/source (`RPending`, `RSuccess`, `RFailed`).
- Config struct field names mirror environment variable names in PascalCase (e.g., `PGSQL_HOSTNAME` maps to `DBConfig.Hostname`).

### Architectural Patterns

- **Dependency injection via struct fields, not globals.** Handler structs (`Export`, `Internal`) receive all dependencies (DB, S3, logger, rate limiter, Kafka function) as fields. This enables testing without DI frameworks.
- **Interface-based abstractions.** `DBInterface` for database, `StorageHandler` for S3. New external dependencies should follow this pattern.
- **Singleton configuration.** `config.Get()` uses `sync.Once`. Never construct `ExportConfig` directly.
- **Singleton logger.** `logger.Get()` returns a `*zap.SugaredLogger`. Enrich per-request with `.With()` and field helpers.
- **Cobra subcommands.** The binary supports `api_server`, `expired_export_cleaner`, and `migrate_db` subcommands. New operational modes should be added as subcommands.
- **Dual API model layers.** API models (`exports/api_models.go`) are separate from DB models (`models/models.go`). Always convert between them using the provided conversion functions. Do not use GORM models directly in JSON responses.
- **Non-blocking Kafka dispatch.** After responding to the client, Kafka messages are sent in goroutines. Follow this fire-and-forget pattern for any new async work triggered by HTTP requests.

### Code Style and Formatting

- CI runs `golangci-lint` (v2.3.0) on all PRs. Code must pass lint before merge.
- Copyright headers are present on all source files:
  ```go
  /*
  Copyright 2022 Red Hat Inc.
  SPDX-License-Identifier: Apache-2.0
  */
  ```
- Error handling follows early-return style: check error, log, respond with HTTP error, `return`. Do not continue execution after writing an error response.
- Use `Errorw` (structured) over `Errorf` (format string) for new logging code.
- JSON error responses are duplicated in two packages (`exports/jsonerror_response.go` and `middleware/jsonerror_response_middleware.go`). Use the one from the package you are in.

### CI and Testing

- **Linting:** `golangci-lint` runs on every push and PR via `.github/workflows/golangci-lint.yaml`.
- **Tests:** Ginkgo test suite runs via `.github/workflows/ginkgo-test.yaml` with `--race`, `--randomize-all`, `--randomize-suites`, coverage uploaded to Codecov.
- **Test command:** `ginkgo -r --race --randomize-all --randomize-suites` (or `make test`).
- **SQL-tagged tests:** `go test ./... -tags=sql -count=1` for tests needing a real (not embedded) Postgres.
- Tests must pass with randomized ordering. Do not rely on test execution order.

### PR and Commit Expectations

- All PRs must pass `golangci-lint` and the Ginkgo test suite.
- OpenAPI spec changes must update both YAML and JSON formats (`make spec` converts YAML to JSON via `yq`).
- New database schema changes require a migration pair in `db/migrations/` -- never use GORM AutoMigrate.
- New handler methods must include rate limiting (`e.RateLimiter.Wait(r.Context())`).
- User-facing DB queries must always be scoped by `User` (account_id, org_id, username).

### Common Pitfalls

- **Do not use `*gorm.DB` directly in handlers.** Always go through `DBInterface` methods.
- **Do not construct `ExportConfig` manually.** Always use `config.Get()`.
- **Do not use `fmt.Printf` or `log.Printf` in production code.** Use the zap logger from `logger.Get()`.
- **Do not add endpoints to the metrics server.** It is unauthenticated and only serves health/readiness/metrics.
- **Do not use `gorm.ErrRecordNotFound` in handler code.** Use the sentinel `models.ErrRecordNotFound` and switch on it.
- **Do not use `First()` for single-record lookups.** Use `Take()` to avoid implicit `ORDER BY`.
- **Do not forget `Preload("Sources")` when you need the child relation.** `Get`/`GetWithUser` preload automatically; `List`/`APIList` do not.
- **S3 keys must use UUIDs and org IDs, never user-supplied strings.** This enforces tenant isolation at the storage level.
- **The Kafka producer channel is unbuffered.** Do not add buffering without measuring backpressure impact.
- **Sort/dir parameters for pagination are injected via `fmt.Sprintf` into SQL.** They are validated against allowlists in middleware. Any new sortable field must be added to the allowlist in `middleware/pagination.go`.
