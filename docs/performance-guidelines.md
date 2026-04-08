# Performance Guidelines

## Configuration Singleton Pattern

The `config.ExportConfig` is initialized exactly once using `sync.Once` in `config.Get()`. Always call `config.Get()` to obtain configuration -- never construct `ExportConfig` manually in production code. This ensures thread-safe, single-allocation access to all runtime settings.

```go
// Correct
cfg := config.Get()

// Wrong -- bypasses sync.Once singleton
cfg := &config.ExportConfig{...}
```

## Rate Limiting

All public-facing API endpoints must apply the shared `rate.Limiter` (from `golang.org/x/time/rate`) before performing any work. The limiter is configured via `RATE_LIMIT_RATE` (default: 100) and `RATE_LIMIT_BURST` (default: 60) environment variables and is created once during server startup.

- Call `e.RateLimiter.Wait(r.Context())` early in the handler, after extracting the user identity and logger but before any database or S3 operations.
- The limiter is a single shared instance across all endpoints on the `Export` struct -- it is not per-user or per-endpoint.
- On rate limit errors, return `InternalServerError` (this is the current convention; the service treats exceeding the limit as an error condition).

## HTTP Server Timeouts

The service runs three separate HTTP servers (public, private, metrics), each with independently configured timeouts:

| Server | Read Timeout Default | Write Timeout Default |
|---------|--------------------|-----------------------|
| Public | 5s | 10s |
| Private | 5s | 10s |
| Metrics | none set | none set |

The private server handles S3 uploads, which may require longer timeouts for large datasets. These are configurable via `PRIVATE_HTTP_SERVER_READ_TIMEOUT` and `PRIVATE_HTTP_SERVER_WRITE_TIMEOUT`. When adjusting, consider the `MAX_PAYLOAD_SIZE` setting (default: 500 MB).

## Payload Size Limits

Incoming upload payloads on the private API are capped using `http.MaxBytesReader`:

```go
maxPayloadSizeBytes := int64(i.Cfg.MaxPayloadSize) * 1024 * 1024
r.Body = http.MaxBytesReader(w, r.Body, maxPayloadSizeBytes)
```

The `MAX_PAYLOAD_SIZE` config is in megabytes (default: 500). This prevents unbounded memory consumption from large uploads. Always apply `MaxBytesReader` before passing the body to S3 upload functions.

## S3 Transfer Buffer Sizing

The AWS S3 uploader and downloader use configurable part sizes for multipart transfers:

- `AWS_UPLOADER_BUFFER_SIZE`: default 10 MB (`10 * 1024 * 1024`)
- `AWS_DOWNLOADER_BUFFER_SIZE`: default 10 MB (`10 * 1024 * 1024`)

These are set once when creating the `s3_manager.Uploader` and `s3_manager.Downloader` at startup. Do not create new uploader/downloader instances per request -- reuse the ones on the `Compressor` struct.

## Kafka Producer Concurrency

The Kafka producer uses a goroutine-per-message pattern:

1. A single unbuffered channel (`chan *kafka.Message`) connects API handlers to the producer.
2. `StartProducer` reads from this channel and spawns a goroutine for each message.
3. Each goroutine creates a dedicated `deliveryChan` for delivery confirmation.
4. Failed messages are re-enqueued onto the same channel for retry.

Key rules:
- The `kafkaProducerMessagesChan` is **unbuffered** -- sends will block if the producer loop is not reading. Do not add buffering without measuring the impact on backpressure.
- The `producerCount` gauge tracks active producer goroutines. Monitor this metric (`export_service_kafka_producer_go_routine_count`) to detect producer goroutine accumulation.
- On shutdown, close the channel first, then call `producer.Flush(1500)` with a 1.5-second timeout before `producer.Close()`.

## Non-blocking Response Pattern

When creating an export, Kafka messages are sent asynchronously so the HTTP response is not blocked:

```go
// In PostExport: respond first, then dispatch to Kafka in background
w.WriteHeader(http.StatusAccepted)
json.NewEncoder(w).Encode(&apiExport)

// RequestAppResources internally launches a goroutine
e.RequestAppResources(r.Context(), logger, identity, *dbExport)
```

The `KafkaRequestApplicationResources` function wraps its work in `go func()`. Similarly, `ProcessSources` launches compression in a goroutine (`go c.compressPayload(...)`) to avoid blocking the upload handler. Follow this pattern for any new background work triggered by an HTTP request.

## Database Access Patterns

### GORM Usage
- Use `gorm.io/gorm` for all database operations. The `ExportDB` struct wraps `*gorm.DB` and implements the `DBInterface` for testability.
- Use `Preload("Sources")` when fetching an `ExportPayload` that needs its associated sources (e.g., `Get`, `GetWithUser`). Omit it for list queries that only return `APIExport` fields.
- Use `Take` (not `First`) for single-record lookups -- `Take` does not add an implicit `ORDER BY`.

### Query Building
- Filter parameters are applied conditionally using chained `.Where()` calls. Only add `JOIN` clauses when filtering by source-level fields (application, resource):

```go
if params.Application != "" || params.Resource != "" {
    db = db.Joins("JOIN sources ON sources.export_payload_id = export_payloads.id")
}
```

- Always call `.Count(&count)` before applying `.Limit()` and `.Offset()` to get the total count for pagination.
- Sort column and direction are injected via `fmt.Sprintf` -- the allowed values are validated in the pagination middleware (`name`, `created`, `expires` for sort; `asc`, `desc` for direction). Never pass unsanitized user input to `Order()`.

### Bulk Deletion
The expired export cleaner uses a single `DELETE ... WHERE` with `clause.Returning` to both delete and retrieve deleted records in one query, avoiding N+1 patterns:

```go
edb.DB.Clauses(clause.Returning{Columns: columnsToReturn}).
    Where(expiredExportsClause).
    Delete(&deletedExports)
```

## Temporary File Management

The S3 compression pipeline downloads files to a temporary directory, zips them in memory, writes the zip to a temp file, then uploads. Always:
- Use `os.MkdirTemp` for the temp directory.
- Use `defer os.RemoveAll(tempDirName)` immediately after creation.
- Pre-allocate slices with known capacity: `make([]s3FileData, 0, len(resp.Contents))`.
- Seek temp files back to offset 0 before passing to the uploader.

## Zip Compression

Use `zip.Deflate` method (not `Store`) when adding files to zip archives. This is set via the file header:

```go
header.Method = zip.Deflate
```

## Prometheus Metrics Conventions

Metrics are registered in `init()` functions across packages. Follow these naming conventions:

| Prefix | Location | Examples |
|--------|----------|---------|
| `export_service_http_*` | `metrics/` | `_requests_total`, `_response_time_seconds` |
| `export_service_kafka_*` | `kafka/` | `_produced`, `_produce_failures` |
| `export_service_*_s3_*` | `s3/` | `_total_s3_uploads`, `_failed_s3_uploads`, `_upload_sizes` |

- Use `CounterVec` with labels for request-type metrics (topic, method, status code).
- Use `HistogramVec` for latency and size distributions.
- Use a plain `Gauge` for tracking active goroutine counts.
- Always register metrics with `prometheus.MustRegister` in `init()`.

## Graceful Shutdown

The API server uses a signal-based graceful shutdown pattern:

1. An `idleConnsClosed` channel coordinates shutdown completion.
2. A goroutine listens for `os.Interrupt` and calls `Shutdown(context.Background())` on all three servers sequentially.
3. After all servers shut down, the Kafka channel is closed and the producer is flushed.

When adding new background resources, register their cleanup in this shutdown sequence -- after servers stop but before the final logger sync.

## Logging Performance

- Use `zap.SugaredLogger` for all application logging (never `fmt.Printf` or `log.Printf` in production paths).
- CloudWatch batch logging is configured with a 10-second flush interval (`lc.NewBatchWriterWithDuration(..., 10*time.Second)`).
- Attach structured fields using `logger.With(field)` to avoid string concatenation. Use the helper functions in `logger/logger.go`: `RequestIDField`, `OrgIDField`, `ExportIDField`, `ApplicationNamesField`.
- The response logger middleware measures latency using `time.Since(t1)` in a deferred function.

## Interface-based Testability

Performance-sensitive components (S3 storage, database) are accessed through interfaces (`StorageHandler`, `DBInterface`). Mock implementations (`MockStorageHandler`) exist in the same package. When adding new storage or database methods:
- Add to the interface first.
- Implement in the concrete struct.
- Add a mock implementation for tests.

This avoids test suites depending on live S3/DB connections, keeping test execution fast.
