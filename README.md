[![codecov](https://codecov.io/gh/RedHatInsights/export-service-go/branch/main/graph/badge.svg?token=6N8SE602A4)](https://codecov.io/gh/RedHatInsights/export-service-go)

# Export Service

The Export Service allows users to request and download data archives for auditing or use with their own tooling. Because many ConsoleDot applications need to export data, this service provides some common functionality.

For more information on integrating with the export service, see the [integration documentation](docs/integration.md).

## Tech Stack

- **Language:** Go 1.24+
- **HTTP Router:** [chi v5](https://github.com/go-chi/chi)
- **ORM:** [GORM](https://gorm.io/) with PostgreSQL driver
- **Database Migrations:** [golang-migrate](https://github.com/golang-migrate/migrate)
- **Messaging:** [confluent-kafka-go](https://github.com/confluentinc/confluent-kafka-go) (CloudEvents schema)
- **Object Storage:** AWS SDK v2 for S3 (MinIO for local development)
- **Configuration:** [Viper](https://github.com/spf13/viper) + [Cobra](https://github.com/spf13/cobra) CLI
- **Logging:** [Zap](https://github.com/uber-go/zap) (SugaredLogger)
- **Metrics:** [Prometheus client_golang](https://github.com/prometheus/client_golang)
- **Testing:** [Ginkgo v2](https://github.com/onsi/ginkgo) + [Gomega](https://github.com/onsi/gomega), embedded Postgres for unit tests
- **Platform:** [Clowder](https://github.com/RedHatInsights/clowder) for deployment on ConsoleDot

## Project Structure

```
cmd/export-service/     # Cobra CLI entrypoint (api_server, expired_export_cleaner, migrate_db)
config/                 # Singleton config via Viper + Clowder
db/                     # Database connection and migration runner
  migrations/           # Sequential SQL migration pairs
exports/                # Public and internal HTTP handlers, API models
kafka/                  # Kafka producer, message types, CloudEvents schema
logger/                 # Zap logger singleton
metrics/                # Prometheus metrics and middleware
middleware/             # Auth (identity, PSK), pagination, URL params
models/                 # GORM models, DBInterface, database access layer
s3/                     # S3 client, StorageHandler interface, upload/download
static/spec/            # OpenAPI specs (YAML and JSON)
utils/                  # Test DB fixture helper
deploy/                 # ClowdApp manifest
docs/                   # Guidelines and integration documentation
```

For detailed conventions and architectural guidance, see [AGENTS.md](AGENTS.md).

## Dependencies
- Golang >= 1.18
- podman-compose
- make (optional for running the Makefile)
- jq (optional for running the Makefile)
- ginkgo

## Starting the service locally
You can start the database, minio, and the api using `make run`. 

Ports now exposed:
- public api on localhost:8000
- metrics on localhost:9090
- internal api on localhost:10010
- minio on localhost:9099
- psql on localhost:5432
(use `minio` as the access key and `minioadmin` as the secret key to view the dashboard)
(use `psql -h localhost -p 5432 -U postgres` and the pass `postgres` to connect to the db)

To test local changes, you can restart the api server using `make run-api`.

## Build and Test

### Building

Build locally (produces the `export-service` binary):

```bash
make build-local
```

Build the container image:

```bash
make build
```

### Running Tests

Run the full Ginkgo test suite (matches CI configuration):

```bash
ginkgo -r --race --randomize-all --randomize-suites
```

Or via Make:

```bash
make test
```

For SQL-tagged integration tests (requires a running Postgres instance):

```bash
go test ./... -tags=sql -count=1
```

### Linting

Code must pass `golangci-lint` before merge. The CI uses v2.3.0:

```bash
golangci-lint run
```

### OpenAPI Spec Updates

When modifying API contracts, regenerate the JSON specs from YAML:

```bash
make spec
```

## Testing the service
You can create a new export request using `make sample-request-create-export` which pulls data from the `example_export_request.json`. It should respond with the following information:
```
{
    "id":"0b069353-6ace-4403-8162-3476df3ae4ab",
    "created_at":"2022-10-12T15:07:12.319191523Z",
    "name":"Example Export Request",
    "format":"json",
    "status":"pending",
    "sources":[{
        "id":"0b1386f4-2b91-44d7-bcb0-9391cfbba4c5","application":"exampleApplication",
        "status":"pending",
        "resource":"exampleResource",
        "filters":{}
    }]
}
```
Replace the `EXPORT_ID` and `EXPORT_RESOURCE` in the `Makefile` with the `id` and the sources `id` from the response.

You can then run `make sample-request-internal-upload` to upload `example_export_upload.zip` to the service. If this is successful, you should be able to download the uploaded file from the service using `make sample-request-export-download`.

## Documentation

| Document | Description |
|----------|-------------|
| [AGENTS.md](AGENTS.md) | Onboarding guide for AI agents and contributors: coding conventions, architecture, common pitfalls |
| [CLAUDE.md](CLAUDE.md) | Claude Code-specific build/test commands and workflow preferences |
| [docs/integration.md](docs/integration.md) | How external applications integrate with the export service |
| [docs/security-guidelines.md](docs/security-guidelines.md) | Auth model, PSK validation, input sanitization, S3 tenant isolation |
| [docs/database-guidelines.md](docs/database-guidelines.md) | PostgreSQL/GORM stack, schema design, migrations, query patterns |
| [docs/api-contracts-guidelines.md](docs/api-contracts-guidelines.md) | OpenAPI specs, request/response formats, Kafka message contract |
| [docs/testing-guidelines.md](docs/testing-guidelines.md) | Ginkgo v2 + Gomega framework, test patterns, build tags |
| [docs/error-handling-guidelines.md](docs/error-handling-guidelines.md) | Logging levels, HTTP error helpers, error wrapping rules |
| [docs/performance-guidelines.md](docs/performance-guidelines.md) | Rate limiting, timeouts, payload limits, Prometheus metrics |
| [docs/integration-guidelines.md](docs/integration-guidelines.md) | Clowder integration, Kafka setup, S3/MinIO usage, local dev |
