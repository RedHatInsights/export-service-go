# Security Guidelines for export-service-go

## Architecture: Dual-Server Security Model

This service runs three separate HTTP servers on different ports, each with its own security posture:

- **Public server** (port 8000): External API at `/api/export/v1` -- requires `X-Rh-Identity` header authentication
- **Private server** (port 10000): Internal API at `/app/export/v1` -- requires `X-Rh-Exports-Psk` header (pre-shared key)
- **Metrics server** (port 9000): Unauthenticated, exposes `/healthz`, `/readyz`, `/metrics` only

Never add authenticated or data-bearing endpoints to the metrics server. Never expose private/internal endpoints on the public server.

## Authentication

### Public API: X-Rh-Identity Header

Authentication is handled by two chained middleware in `cmd/export-service/api_server.go`:

1. `identity.EnforceIdentity` (from `platform-go-middlewares/v2/identity`) -- decodes and validates the base64-encoded `X-Rh-Identity` header
2. `emiddleware.EnforceUserIdentity` (from `middleware/user.go`) -- extracts and validates `AccountID`, `OrganizationID`, and `Username` from the identity

Three identity types are supported: `User`, `ServiceAccount`, and `System` (cert-auth). Each must provide a non-empty username (or `CommonName` for cert-auth). Any new identity type must be explicitly handled in `getUsernameFromIdentityHeader()` in `middleware/user.go`; unknown types are rejected.

Always retrieve the authenticated user via `middleware.GetUserIdentity(r.Context())`. The `X-Rh-Identity` header may be accessed directly only when forwarding it to downstream services (e.g., in Kafka message headers), never for authentication purposes in handler code.

### Private/Internal API: Pre-Shared Keys

Internal endpoints are protected by the `EnforcePSK` middleware (`middleware/psks.go`). It checks the `X-Rh-Exports-Psk` header against the configured list in `config.Psks` (sourced from the `EXPORTS_PSKS` environment variable / Kubernetes secret `export-service-psks`).

Rules enforced:
- Exactly one `X-Rh-Exports-Psk` header value must be present (multiple values = 400)
- The value must match one of the configured PSKs (mismatch = 401)

The PSK secret is marked `optional: true` in the ClowdApp manifest. When adding new internal endpoints, always place them under the `/app/export/v1` route which has `EnforcePSK` applied.

## Authorization: User-Scoped Data Access

All database queries for user-facing endpoints must be scoped to the authenticated user. The `User` struct (containing `AccountID`, `OrganizationID`, `Username`) is used as a GORM `Where` clause:

```go
// Correct: scoped to user
edb.DB.Where(&ExportPayload{ID: exportUUID, User: user}).Delete(&ExportPayload{})
edb.DB.Where(&ExportPayload{User: user}).Find(&result)

// Wrong: no user scope
edb.DB.Where(&ExportPayload{ID: exportUUID}).Delete(&ExportPayload{})
```

Use `DB.GetWithUser()` for user-facing reads and `DB.Get()` only for internal/PSK-authenticated operations. The database has a composite index on `(account_id, organization_id, username)` to support this pattern.

## SQL Injection Prevention

- Use GORM's parameterized queries for all user-controlled input. The codebase uses `db.Where("column = ?", value)` syntax consistently in `models/db.go`.
- Raw SQL via `db.Raw()` must always use parameterized placeholders. See `SetSourceStatus()` in `models/models.go` for the correct pattern:
  ```go
  sql = db.Raw("UPDATE sources SET status = ?, code = ?, message = ? WHERE id = ?",
      status, sourceError.Code, sourceError.Message, uid)
  ```
- The `sort` and `dir` pagination parameters are validated against allowlists (`name`, `created`, `expires` for sort; `asc`, `desc` for dir) in `middleware/pagination.go` before being interpolated into `ORDER BY` clauses. Any new sortable field must be added to this allowlist.

## Input Validation

### UUID Validation
All URL path parameters representing UUIDs (`exportUUID`, `resourceUUID`) are parsed through `uuid.Parse()` in `middleware/internal.go` (via `URLParamsCtx` middleware) or directly in handlers. Invalid UUIDs return 400. Never use raw string URL params as database keys.

### Request Body Validation
- JSON request bodies are decoded with `json.NewDecoder(r.Body).Decode()` -- malformed JSON returns 400
- Export sources must be non-empty (`len(apiExport.Sources) == 0` check)
- Source filters are validated as valid JSON via `json.Unmarshal` in `APIExportToDBExport()`
- Export format is validated against an explicit allowlist (`csv`, `json`; default returns error)
- Application and resource names are validated against the configured `ExportableApplications` map via `verifyExportableApplication()`

### Pagination Parameters
The `PaginationCtx` middleware validates `limit`, `offset`, `sort`, and `dir` query parameters. Negative values for `limit` and `offset` are rejected. Sort/dir values are restricted to explicit allowlists.

## Payload Size Limits

Upload payloads to the internal API are limited using `http.MaxBytesReader`:

```go
maxPayloadSizeBytes := int64(i.Cfg.MaxPayloadSize) * 1024 * 1024
r.Body = http.MaxBytesReader(w, r.Body, maxPayloadSizeBytes)
```

Default is 500 MB, configurable via `MAX_PAYLOAD_SIZE` env var. Exceeding this limit returns 413.

## HTTP Server Hardening

- **Read/Write timeouts** are configured on all three servers (defaults: 5s read, 10s write for public; configurable via env vars). Never set timeouts to zero.
- **Content-Type enforcement**: The `JSONContentType` middleware sets `Content-Type: application/json` globally. The `GZIPContentType` middleware is applied specifically to the export download endpoint.
- **X-Content-Type-Options**: The `nosniff` header is set on all JSON error responses via the `JSONError()` function in both `middleware/jsonerror_response_middleware.go` and `exports/jsonerror_response.go`.
- **Panic recovery**: `middleware.Recoverer` from chi is applied to both public and private routers.

## Rate Limiting

The public API uses `golang.org/x/time/rate.Limiter` (token bucket algorithm) configured via:
- `RATE_LIMIT_RATE` (default: 100 requests/second)
- `RATE_LIMIT_BURST` (default: 60)

Rate limiting is applied per-handler (not per-user) via `e.RateLimiter.Wait(r.Context())`. Every public handler that performs database or S3 operations must include this call.

## Secrets Management

- **Database credentials**: Sourced from Clowder (`cfg.Database`) in production; from env vars in development. Never hardcode credentials.
- **PSKs**: Loaded from Kubernetes secret `export-service-psks` (key: `psk-list`), comma-separated. The secret is `optional: true`.
- **S3/Object Store credentials**: Sourced from Clowder's object store configuration in production.
- **Kafka SASL credentials**: Conditionally configured only when `broker.Authtype` is set in Clowder config.
- **Logging credentials**: CloudWatch access keys are sourced from Clowder.

Never log secrets. The `startApiServer` function in `api_server.go` logs configuration values but excludes credentials -- maintain this pattern.

## Database Security

- **SSL/TLS**: Database connections support SSL via `sslmode` parameter and optional RDS CA certificate (`sslrootcert`). In Clowder environments, SSL mode is set from `cfg.Database.SslMode`.
- **Migrations**: Use golang-migrate with versioned SQL files in `db/migrations/`. Always use parameterized queries in migrations.
- **Cascade deletes**: Sources are deleted via `ON DELETE CASCADE` when their parent `export_payload` is removed.
- **Automatic expiry**: The `expired_export_cleaner` job runs on a cron schedule and deletes exports past their expiry window (`EXPORT_EXPIRY_DAYS`, default 7 days).

## S3 Object Key Structure

S3 keys are constructed using organization-scoped paths to enforce tenant isolation at the storage level:

```
{organization_id}/{export_id}/{resource_uuid}.{format}   # individual resources
{organization_id}/{timestamp}-{export_id}.zip             # final export
```

Never construct S3 keys from user-supplied strings. Always use UUIDs and organization IDs from the authenticated identity.

## Kafka Security

- Kafka connections support SASL/SSL when configured through Clowder
- The `X-Rh-Identity` header is forwarded to downstream applications via Kafka message headers (`IDheader`), preserving the authentication chain
- Kafka messages use CloudEvents schema with structured types from `event-schemas-go`

## Testing Security Controls

Security middleware is tested with table-driven tests using Ginkgo/Gomega (see `middleware/user_test.go` and `middleware/psks_test.go`). When adding new security middleware:

- Test with missing headers (expect 400)
- Test with invalid/malformed values (expect 400 or 401)
- Test with null/empty nested objects (handle nil pointer cases)
- Test that the handler is NOT called when middleware rejects the request
- Use `httptest.NewRecorder()` and real chi routers to test the full middleware chain

## Container Security

- The Dockerfile uses a multi-stage build with `ubi9-minimal` as the runtime base
- The final image runs as non-root user (`USER 1001`)
- Binary is built with `-ldflags "-w -s"` to strip debug information
