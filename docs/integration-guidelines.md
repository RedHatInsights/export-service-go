# Integration Guidelines for export-service-go

## Architecture Overview

The service runs three HTTP servers concurrently from a single binary (via cobra subcommands):
- **Public server** (port 8000): User-facing API at `/api/export/v1`, authenticated via `X-Rh-Identity`
- **Private server** (port 10000): Internal API at `/app/export/v1`, authenticated via PSK (`X-Rh-Exports-Psk`)
- **Metrics server** (port 9000): Prometheus metrics, health/readiness probes at `/healthz` and `/readyz`

A separate `expired_export_cleaner` subcommand runs as a scheduled CronJob.

## Configuration Management

- All configuration flows through `config/config.go` using a singleton pattern (`sync.Once` + `config.Get()`).
- Use `viper` with `AutomaticEnv()` for environment variable binding. Defaults are set via `options.SetDefault()`.
- When Clowder is enabled (`clowder.IsClowderEnabled()`), all infrastructure config (DB, Kafka, S3, CloudWatch) is overridden from `clowder.LoadedConfig`. Never hardcode connection details for these services.
- The `EXPORT_ENABLE_APPS` env var is a JSON map controlling which application/resource pairs are allowed. New integrating applications must be added here.

## Clowder Platform Integration

- Clowder provides: database credentials, Kafka broker list and topic names, object store endpoints/credentials, CloudWatch logging config, and port assignments.
- Kafka topic names are resolved via `clowder.KafkaTopics[ExportTopic].Name` where `ExportTopic = "platform.export.requests"`. The Clowder-mapped name may differ from the logical name.
- Object bucket is resolved via `clowder.ObjectBuckets[exportBucket]` using the `EXPORT_SERVICE_BUCKET` env var.
- Kafka SSL/SASL config is conditionally applied only when `broker.Authtype != nil` or `broker.Cacert != nil`.
- DB SSL uses RDS CA from Clowder when `cfg.Database.RdsCa != nil`.
- The ClowdApp spec is in `deploy/clowdapp.yaml`. DB migrations run as an init container (`export-service migrate_db upgrade`).

## Kafka Integration

- **Library**: `confluent-kafka-go` (CGo-based, not pure Go).
- **Producer only** -- this service does not consume Kafka messages.
- Messages are sent through a `chan *kafka.Message` channel. The producer goroutine reads from this channel and publishes asynchronously, one goroutine per message with a delivery channel.
- Failed deliveries are re-enqueued onto the same channel (`msgChan <- msg`).
- On shutdown, the channel is closed, then `producer.Flush(1500)` is called before `producer.Close()`.

### Kafka Message Format

Messages follow the CloudEvents spec using `github.com/RedHatInsights/event-schemas-go/apps/exportservice/v1`:

```go
KafkaMessage{
    ID:          uuid.New(),
    Schema:      kafkaConfig.EventSchema,
    Source:      kafkaConfig.EventSource,        // "urn:redhat:source:console:app:export-service"
    Subject:     "urn:redhat:subject:export-service:request:<exportID>",
    SpecVersion: kafkaConfig.EventSpecVersion,   // "1.0"
    Type:        kafkaConfig.EventType,
    Data:        cloudEventSchema.ResourceRequest{...},
}
```

Headers include `application` (target app name) and `x-rh-identity` (base64 identity header).

### Kafka Message Dispatch Pattern

The `RequestApplicationResources` function type is a closure that captures the Kafka channel. It is called as a goroutine after the HTTP response is sent (non-blocking):

```go
e.RequestAppResources(r.Context(), logger, r.Header["X-Rh-Identity"][0], *dbExport)
```

One Kafka message is produced per source in the export request. Each source targets a specific application/resource.

## S3/MinIO Integration

- **Library**: `aws-sdk-go-v2` (not v1). S3 client is created with `s3.NewFromConfig` with `UsePathStyle: true` (required for MinIO compatibility).
- Local dev uses MinIO on port 9099 with bucket `exports-bucket` auto-created via `minio/mc` in docker-compose.
- The `Compressor` struct is the central storage abstraction, holding the S3 client, uploader, and downloader with configurable buffer sizes (`AWS_UPLOADER_BUFFER_SIZE`, `AWS_DOWNLOADER_BUFFER_SIZE`, default 10MB).
- The `StorageHandler` interface defines the contract: `Compress`, `Download`, `Upload`, `CreateObject`, `GetObject`, `ProcessSources`.
- S3 key format: `<orgID>/<exportID>/<resourceUUID>.<format>` for individual uploads; `<orgID>/<timestamp>-<exportID>.zip` for final archives.
- On upload failure, the partially uploaded object is deleted (`DeleteObject`).
- Compression packages all source files into a ZIP archive with `meta.json` and `README.md` metadata files.

## CloudWatch Integration

- Logging uses `go.uber.org/zap` with a `SugaredLogger` singleton (`logger.Get()`).
- CloudWatch is configured as an additional `zapcore.Core` tee when `cfg.Logging.Region != ""`.
- Uses `platform-go-middlewares/v2/logging/cloudwatch.NewBatchWriterWithDuration` with a 10-second batch interval.
- Credentials come from Clowder's `cfg.Logging.Cloudwatch` fields.

## Structured Logging Conventions

Always enrich loggers with context fields using the helper functions from `logger/logger.go`:

```go
logger := e.Log.With(
    export_logger.RequestIDField(reqID),
    export_logger.OrgIDField(user.OrganizationID),
    export_logger.ExportIDField(exportUUID),
    export_logger.ApplicationNamesField(appNames),
)
```

Use `Infow`, `Errorw`, `Debugw` (structured) rather than `Infof`, `Errorf` (format string) for new code.

## Prometheus Metrics

- Register metrics in `init()` functions using `prometheus.MustRegister`.
- All metric names are prefixed with `export_service_`.
- HTTP metrics: `export_service_http_requests_total` (counter by code/method), `export_service_http_response_time_seconds` (histogram by method).
- Kafka metrics: `export_service_kafka_produced`, `export_service_publish_seconds`, `export_service_kafka_produce_failures`, `export_service_kafka_producer_go_routine_count`.
- S3 metrics: `export_service_total_s3_uploads`, `export_service_failed_s3_uploads`, `export_service_upload_sizes` (histogram by app).

## Database Integration

- **ORM**: GORM with PostgreSQL driver. Migrations use `golang-migrate/migrate/v4` with SQL files in `db/migrations/`.
- The `DBInterface` interface in `models/db.go` defines all DB operations. Use this interface (not raw `*gorm.DB`) in handler structs to enable mocking.
- Models use UUID primary keys generated in `BeforeCreate` hooks. Sources are a child table with `foreignKey:ExportPayloadID`.
- Status transitions: `Pending -> Running -> Complete|Partial|Failed`. Source statuses: `RPending -> RSuccess|RFailed`.

## Authentication and Middleware

- **Public API**: Uses `identity.EnforceIdentity` (from `platform-go-middlewares/v2`) to decode `X-Rh-Identity`, then `EnforceUserIdentity` to extract `AccountID`, `OrganizationID`, `Username` into context.
- **Private API**: Uses `EnforcePSK` middleware checking `X-Rh-Exports-Psk` against configured PSKs from `EXPORTS_PSKS` secret.
- Supports identity types: `User`, `ServiceAccount`, `System` (cert-based).
- URL parameters (`exportUUID`, `application`, `resourceUUID`) are parsed and validated in `URLParamsCtx` middleware, stored in context.

## Testing Patterns

- Test framework: Ginkgo v2 + Gomega.
- Tests use `embedded-postgres` (`fergusstrange/embedded-postgres`) for a real PostgreSQL instance on port 5432.
- DB setup is in `utils/fixtures.go` via `CreateTestDB()` which starts embedded postgres and runs migrations.
- Kafka is mocked by replacing `RequestApplicationResources` with a function literal (not a mock library):

```go
mockKafkaCall := func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
    wasKafkaMessageSent = true
}
```

- S3 is mocked via `MockStorageHandler` (defined in `s3/compress.go`) which implements the `StorageHandler` interface.
- Test identity headers use a hardcoded base64-encoded `X-Rh-Identity` string set via `AddDebugUserIdentity(req)`.
- Each test cleans the DB with `testGormDB.Exec("DELETE FROM export_payloads")` in `BeforeEach`.

## Router Structure

Uses `go-chi/chi/v5`. Route registration pattern:

```go
router.Route("/api/export/v1", func(r chi.Router) {
    r.Use(identity.EnforceIdentity, emiddleware.EnforceUserIdentity)
    r.Route("/exports", external.ExportRouter)
})
```

Handler structs (`Export`, `Internal`) own their dependencies and define router methods (`ExportRouter`, `InternalRouter`).

## Dependency Injection

All external dependencies are injected through handler struct fields -- not globals:

```go
type Export struct {
    Bucket              string
    StorageHandler      es3.StorageHandler    // interface
    DB                  models.DBInterface    // interface
    RequestAppResources RequestApplicationResources // function type
    Log                 *zap.SugaredLogger
    RateLimiter         *rate.Limiter
}
```

This pattern enables test mocking without dependency injection frameworks.

## Local Development

Use `docker-compose.yaml` which provides: PostgreSQL 14 (port 5432), MinIO (port 9099), Zookeeper + Kafka (port 29092). The `s3-createbucket` service auto-creates the `exports-bucket`. Default credentials: DB `postgres/postgres`, MinIO `minio/minioadmin`, PSK `testing-a-psk`.
