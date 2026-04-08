# API Contract Guidelines - export-service-go

## OpenAPI Specification

The service maintains two OpenAPI 3.0.1 specs as the source of truth for all API contracts:

- **Public API**: `static/spec/openapi.yaml` (also served as `openapi.json`) -- exposed at `/api/export/v1/openapi.json`
- **Private/Internal API**: `static/spec/private.yaml` (also served as `private.json`) -- exposed at `/app/export/v1/openapi.json`

Both YAML and JSON versions exist in `static/spec/`. Any endpoint change must update both formats. The specs are served at runtime via `http.ServeFile` from paths configured in `config.go` (`OPEN_API_FILE_PATH` and `OPEN_API_PRIVATE_PATH`).

## Dual-Server Architecture

The service runs three HTTP servers on separate ports:

| Server | Base Path | Default Port | Auth Mechanism |
|---------|-----------|------|------|
| Public | `/api/export/v1` | 8000 | `X-Rh-Identity` header (base64-encoded JSON, enforced by `identity.EnforceIdentity`) |
| Private | `/app/export/v1` | 10000 | `X-Rh-Exports-Psk` header (pre-shared key) |
| Metrics | `/` | 9000 | None (`/metrics`, `/healthz`, `/readyz`) |

All public endpoints require the `X-Rh-Identity` header. The middleware extracts `account_number`, `org_id`, and `username` and injects them into the request context. The private API uses a PSK validated against a configured allowlist (`EXPORTS_PSKS` env var).

## URL and Versioning Conventions

- Public API version prefix: `/api/export/v1`
- Private API version prefix: `/app/export/v1`
- Path parameters use `{exportUUID}`, `{application}`, `{resourceUUID}` naming
- UUIDs in paths are validated using `uuid.Parse()` from `github.com/google/uuid`, which enforces RFC 4122 format
- The router is `go-chi/chi/v5`. Routes are defined in `ExportRouter()` and `InternalRouter()` methods.

## Request/Response Format

### Content Types

- Default response `Content-Type` is `application/json` (set globally via `JSONContentType` middleware)
- Export downloads return `application/gzip` (set via `GZIPContentType` middleware on the download route)
- The private upload endpoint accepts `application/json` or `application/csv`

### Error Response Shape

All error responses use this consistent JSON structure, defined identically in both `exports/jsonerror_response.go` and `middleware/jsonerror_response_middleware.go`:

```json
{"message": "<string or object>", "code": <http_status_code>}
```

Use the helper functions: `BadRequestError`, `InternalServerError`, `NotFoundError`, `NotImplementedError`, `StatusNotAcceptableError`. The `code` field mirrors the HTTP status code. The `message` field type is `interface{}` -- it can be a string or a structured error.

### Source Error Shape (Internal API)

When a source application reports an error, the body must contain both fields:

```json
{"message": "human-readable error", "error": 404}
```

The `error` field is an integer HTTP status code (400-599). Both `message` and `error` are required.

## Pagination

Pagination is implemented via the `PaginationCtx` middleware (applied only to `GET /exports`). It follows offset/limit style:

| Parameter | Default | Validation |
|-----------|---------|------------|
| `limit` | 100 | Must be >= 0 |
| `offset` | 0 | Must be >= 0 |
| `sort` | `created_at` | Allowed: `name`, `created` (maps to `created_at`), `expires` |
| `dir` | `asc` | Allowed: `asc`, `desc` |

Note: the OpenAPI spec defines `sort` values as `name`, `created_at`, `expires_at`, but the middleware accepts `created` (which it maps to `created_at`) and `expires` (which it uses directly against the database column `expires`, not `expires_at`). When filtering or sorting by expiration, use `expires` in the query parameter, which matches the actual database column name.

The paginated response envelope:

```json
{
  "meta": {"count": <total_records>},
  "links": {
    "first": "<url>",
    "next": "<url or null>",
    "previous": "<url or null>",
    "last": "<url>"
  },
  "data": [...]
}
```

`next` and `previous` are nullable (nil pointer in Go produces JSON `null`).

## Query Filtering

The `GET /exports` endpoint supports these optional query filters, parsed in `exports/utils.go`:

- `name` -- exact match
- `status` -- exact match on export status
- `created_at` -- date in ISO 8601 (`YYYY-MM-DD` or `YYYY-MM-DDThh:mm:ssZ`)
- `expires_at` -- same date format as `created_at`
- `application` -- filters by joining the `sources` table
- `resource` -- filters by joining the `sources` table

Date filtering uses a one-day range (`BETWEEN date AND date+1day`).

## Status Enums

### Export (Payload) Status

Defined as `PayloadStatus` in `models/models.go`: `pending`, `running`, `partial`, `complete`, `failed`.

New exports default to `pending`.

### Resource (Source) Status

Defined as `ResourceStatus` in `models/models.go`: `pending`, `success`, `failed`.

New sources default to `pending`.

### Format

Defined as `PayloadFormat`: `json`, `csv`. These are the only valid values; any other format returns a 400 error.

## Kafka Message Contract

Messages are published to the `platform.export.requests` topic (configurable via `KAFKA_ANNOUNCE_TOPIC`). The message follows the CloudEvents specification:

```json
{
  "id": "<new UUID>",
  "$schema": "<KAFKA_EVENT_SCHEMA>",
  "source": "urn:redhat:source:console:app:export-service",
  "subject": "urn:redhat:subject:export-service:request:<export_id>",
  "specversion": "1.0",
  "type": "com.redhat.console.export-service.request",
  "time": "<ISO 8601 UTC>",
  "redhatorgid": "<org_id>",
  "dataschema": "<KAFKA_EVENT_DATASCHEMA>",
  "data": {
    "resource_request": {
      "application": "<app_name>",
      "export_request_uuid": "<export_id>",
      "filters": {},
      "format": "json|csv",
      "resource": "<resource_name>",
      "uuid": "<source_id>",
      "x_rh_identity": "<identity_header>"
    }
  }
}
```

Kafka headers include `application` and `x-rh-identity`. The data schema type comes from `github.com/RedHatInsights/event-schemas-go/apps/exportservice/v1`.

One message is produced per source in the export request. Messages are sent asynchronously via a goroutine after the HTTP response is returned.

## API Model Layers

The codebase maintains separate model layers:

- **API models** (`exports/api_models.go`): `ExportPayload`, `Source`, `SourceError` -- used for JSON serialization with `json` struct tags
- **DB models** (`models/models.go`): `ExportPayload`, `Source`, `SourceError`, `User` -- used with GORM, includes `gorm` struct tags
- **List API model** (`models/db.go`): `APIExport` -- a subset of fields returned by `GET /exports` list endpoint

Conversion functions `DBExportToAPI()` and `APIExportToDBExport()` in `exports/exports.go` handle translation. All timestamps are converted to UTC before API serialization. The `omitempty` tag is used on nullable time fields (`completed_at`, `expires_at`).

## Exportable Applications Allowlist

The `EXPORT_ENABLE_APPS` env var defines which application/resource combinations are valid. Requests for unlisted applications or resources receive a `406 Not Acceptable` response. The allowlist is a JSON map: `{"appName": ["resource1", "resource2"]}`.

## Rate Limiting

All public and private endpoints use `golang.org/x/time/rate.Limiter` (token bucket). Defaults: rate=100, burst=60. Configured via `RATE_LIMIT_RATE` and `RATE_LIMIT_BURST`. Rate limit exhaustion returns a 500 error.

## Internal API Flow

The private API has two endpoints per export source:

- `POST /{exportUUID}/{application}/{resourceUUID}/upload` -- source app uploads exported data (stored in S3)
- `POST /{exportUUID}/{application}/{resourceUUID}/error` -- source app reports a failure

Both return `202 Accepted` on success. If a source has already been processed (`success` or `failed` status), the endpoint returns `410 Gone`. Upload payloads are limited to `MAX_PAYLOAD_SIZE` MB (default 500), enforced via `http.MaxBytesReader`, returning `413 Request Entity Too Large` when exceeded.
