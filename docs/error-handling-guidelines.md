# Error Handling Guidelines for export-service-go

## Logging Library

This repo uses **zap** (`go.uber.org/zap`) via a global `*zap.SugaredLogger`. Access it through `logger.Get()`.

- The `exports/jsonerror_response.go` package caches the logger in a package-level `var log = logger.Get()`.
- In handler structs (`Export`, `Internal`), the logger is a `Log *zap.SugaredLogger` field, allowing per-request enrichment with contextual fields.

## Structured Log Fields

Always enrich loggers with contextual fields before logging errors. Use the field constructors from the `logger` package:

```go
logger := e.Log.With(
    export_logger.RequestIDField(reqID),
    export_logger.OrgIDField(user.OrganizationID),
)
// After obtaining an export ID, add it too:
logger = logger.With(export_logger.ExportIDField(uid))
```

Available field helpers:
- `logger.RequestIDField(string)` -- `"request_id"`
- `logger.OrgIDField(string)` -- `"org_id"`
- `logger.ExportIDField(string)` -- `"export_id"`
- `logger.ApplicationNamesField([]string)` -- `"application_names"`

## Log Levels for Errors

The repo uses specific levels depending on severity:

| Level | When to use | Example |
|-------|-------------|---------|
| `Errorw` | Operational failures that require attention | DB errors, S3 upload failures, rate limit hits |
| `Warnw` | Non-critical write failures | `Logerr` logs `http.ResponseWriter.Write` failures as warnings |
| `Debugw` | Expected "not found" conditions on internal endpoints | `logger.Debugw("export not found", "error", err)` |
| `Infof` | Expected "not found" on public endpoints, status messages | `logger.Infof("record '%s' not found", exportUUID)` |
| `Panic` / `Panicw` | Unrecoverable startup failures | Failed DB connection, failed Kafka producer creation |

The codebase uses both `Errorw` (structured) and `Errorf` (formatted) in roughly equal proportions. Prefer `Errorw` with `"error", err` for new code as it provides better structured logging.

## HTTP Error Response Pattern

All HTTP error responses use the JSON error helpers defined in **two places** (they are duplicated):
- `exports/jsonerror_response.go` -- used by handler code in `exports/`
- `middleware/jsonerror_response_middleware.go` -- used by middleware code

Both define identical `Error` struct and `JSONError` function:

```go
type Error struct {
    Msg  interface{} `json:"message"`
    Code int         `json:"code"`
}
```

Response format is always `{"message": "...", "code": 400}`.

### Available error response helpers

| Function | Status Code | Package |
|----------|-------------|---------|
| `BadRequestError(w, err)` | 400 | `exports`, `middleware` |
| `NotFoundError(w, err)` | 404 | `exports` |
| `StatusNotAcceptableError(w, err)` | 406 | `exports` |
| `InternalServerError(w, err)` | 500 | `exports` |
| `NotImplementedError(w)` | 501 | `exports` |
| `JSONError(w, err, code)` | any | `exports`, `middleware` |

### Rules for error message content

- For **400 Bad Request**: pass the user-facing message as a string, often `err.Error()`.
- For **500 Internal Server Error**: pass the raw `err` (the `interface{}` type allows it). Do NOT expose raw errors for 400s -- use `.Error()` or a custom string.
- For **404 Not Found**: use a formatted string like `fmt.Sprintf("record '%s' not found", exportUUID)`.
- For **406 Not Acceptable**: use a static string like `"Payload does not match Configured Exports"`.

## Handler Error Flow

Every handler follows this pattern:

1. Extract request ID and user identity from context.
2. Build an enriched logger.
3. For each operation that can fail: log the error with `Errorw`, then call the appropriate HTTP error helper and `return`.

```go
err := someOperation()
if err != nil {
    logger.Errorw("description of what failed", "error", err)
    InternalServerError(w, err)
    return
}
```

Prefer to `return` after writing an error response. There are a few places in the codebase where this pattern is violated (code continues after error response when `w.WriteHeader()` has already been called), but these should be avoided in new code.

## Sentinel Errors and Switch-Based Dispatch

The repo defines one sentinel error:

```go
var ErrRecordNotFound = errors.New("record not found")  // models/db.go
```

The DB layer translates `gorm.ErrRecordNotFound` into `models.ErrRecordNotFound` using `errors.Is()`. Handler code then dispatches on it with a `switch` statement (not `errors.Is`):

```go
switch err {
case models.ErrRecordNotFound:
    NotFoundError(w, fmt.Sprintf("record '%s' not found", exportUUID))
    return
default:
    logger.Errorw("error querying for payload entry", "error", err)
    InternalServerError(w, err)
    return
}
```

Follow this same switch pattern when handling DB errors. Use `models.ErrRecordNotFound` for 404 responses.

## Error Wrapping

Use `fmt.Errorf("context: %w", err)` for wrapping in non-handler code (models, S3, utilities). Examples from the codebase:

```go
return nil, fmt.Errorf("failed to get sources: %w", err)
return nil, fmt.Errorf("failed to upload zip file `%s` to s3: %w", s3key, err)
return nil, fmt.Errorf("invalid json format of filters")  // no wrapping for validation errors
```

Do NOT wrap errors in handler code -- log and respond instead.

## The Logerr Helper

Use `exports.Logerr()` when calling `w.Write()` or `fmt.Fprintf(w, ...)` where you need to discard the byte count but handle the error:

```go
Logerr(w.Write(buf.Bytes()))
Logerr(fmt.Fprintf(w, "payload is too large, max size: %dMB", cfg.MaxPayloadSize))
```

## errors.As for Type Assertion

Use `errors.As` for checking specific error types (not type assertion). The repo uses this for `http.MaxBytesError`:

```go
var maxBytesError *http.MaxBytesError
if errors.As(err, &maxBytesError) {
    w.WriteHeader(http.StatusRequestEntityTooLarge)
}
```

## Startup / Fatal Errors

Use `log.Panic` for unrecoverable errors during startup:

```go
log.Panic("failed to create kafka producer", "error", err)
log.Panic("failed to open database", "error", err)
```

The `middleware.Recoverer` from chi handles panics in HTTP handlers, so panics during request handling are caught. But `log.Panic` is reserved for startup code only.

## Goroutine Error Handling

Errors in goroutines are logged but not propagated. The pattern is log-and-return:

```go
go func() {
    sources, err := payload.GetSources()
    if err != nil {
        log.Errorw("failed unmarshalling sources", "error", err)
        return
    }
    // ... continue processing, use `continue` to skip individual items
}()
```

When a goroutine processes multiple items (e.g., Kafka messages for sources), use `continue` to skip failed items rather than aborting the entire loop.

## Metrics for Error Tracking

Errors are tracked via Prometheus counters, not custom error types:

| Metric | Package | What it tracks |
|--------|---------|----------------|
| `export_service_http_requests_total` | `metrics` | All HTTP requests by status code and method |
| `export_service_failed_s3_uploads` | `s3` | Failed S3 uploads |
| `export_service_total_s3_uploads` | `s3` | Total S3 upload attempts |
| `export_service_kafka_produce_failures` | `kafka` | Failed Kafka message productions |

Increment failure counters at the point of failure, alongside error logging.

## Deferred Cleanup Errors

Log cleanup errors but do not propagate them. Use `Errorf` in deferred functions:

```go
defer func() {
    if err := os.RemoveAll(tempDirName); err != nil {
        logger.Errorf("warning: failed to remove temporary directory %s: %v", tempDirName, err)
    }
}()
```

## Middleware Validation Errors

Middleware that validates request parameters (PSK, identity, pagination, URL params) should call `BadRequestError` or `JSONError` directly and return without calling `next.ServeHTTP`:

```go
if !SliceContainsString(Cfg.Psks, psk[0]) {
    JSONError(w, "invalid x-rh-exports-psk header", http.StatusUnauthorized)
    return
}
```

## Test Error Assertions

Tests use Ginkgo/Gomega. Assert errors with:

```go
Expect(err).ShouldNot(HaveOccurred())  // for no-error cases
Expect(err).To(BeNil())                // alternative form used in the repo
Expect(rr.Code).To(Equal(http.StatusBadRequest))
Expect(rr.Body.String()).To(ContainSubstring("expected error message"))
```

Validate both the HTTP status code and the error message substring in the response body.
