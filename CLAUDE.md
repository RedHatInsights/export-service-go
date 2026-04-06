# Claude Code Configuration

@AGENTS.md

This file contains Claude Code-specific operational instructions for the export-service-go repository. For coding conventions, architectural context, and domain-specific guidelines, see `AGENTS.md` and the files it references.

## Build and Test Commands

### Running Tests

Before suggesting a PR or committing changes, always run the test suite:

```bash
ginkgo -r --race --randomize-all --randomize-suites
```

This matches the CI configuration (`.github/workflows/ginkgo-test.yaml`) which runs:
- Race detection (`--race`)
- Randomized test order (`--randomize-all --randomize-suites`)
- 5-minute timeout
- Coverage reporting

For SQL-tagged integration tests (requires running Postgres):
```bash
go test ./... -tags=sql -count=1
```

### Linting

Code must pass `golangci-lint` before merge. Run locally:

```bash
golangci-lint run
```

The CI uses `golangci-lint` v2.3.0 (`.github/workflows/golangci-lint.yaml`). The configuration (`.golangci.yaml`) enables:
- `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`
- Auto-fix mode
- `gofumpt` and `goimports` formatters with local import prefix `github.com/redhatinsights/export-service-go`

### Pre-commit Hooks

The repository uses pre-commit hooks (`.pre-commit-config.yaml`) with `golangci-lint` v1.64.5. If you make changes, the pre-commit hook will run automatically on commit.

### OpenAPI Spec Updates

When modifying API contracts, update both YAML and JSON formats:

```bash
make spec
```

This converts `static/spec/openapi.yaml` and `static/spec/private.yaml` to their `.json` equivalents using `yq`.

### Local Development

Start the full local stack (DB, MinIO, Kafka, API server):

```bash
make run
```

Restart just the API server after code changes:

```bash
make run-api
```

Build locally without containers:

```bash
make build-local
```

## CI Expectations

All PRs must:
1. Pass `golangci-lint` v2.3.0
2. Pass the full Ginkgo test suite with race detection
3. Include database migrations (never use GORM AutoMigrate)
4. Update OpenAPI specs (both YAML and JSON) if API contracts change

## Claude Code Workflow Preferences

- **Always run tests before suggesting a PR.** Use `ginkgo -r --race --randomize-all --randomize-suites`.
- **Run lint checks** with `golangci-lint run` to catch issues early.
- **When modifying OpenAPI specs**, run `make spec` to sync YAML and JSON.
- **For database schema changes**, create migration pairs in `db/migrations/` with the format `NNNNNN_description.{up,down}.sql`.
- **Test randomization matters**: Tests must pass in any order. Do not introduce ordering dependencies.
