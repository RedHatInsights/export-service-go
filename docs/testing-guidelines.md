# Testing Guidelines for export-service-go

## Test Framework

This project uses **Ginkgo v2** (`github.com/onsi/ginkgo/v2`) with **Gomega** (`github.com/onsi/gomega`) as the assertion library. Do not use the standard `testing` assertions or `testify`. All test files must use Ginkgo's BDD-style constructs.

## Running Tests

```bash
# Run all tests (preferred)
ginkgo -r --race --randomize-all --randomize-suites

# Run SQL-tagged integration tests (requires a running Postgres instance)
go test ./... -tags=sql -count=1
```

## Test File Structure

### Suite Files

Every package with tests must have a `*_suite_test.go` file that bootstraps the Ginkgo runner. The suite file follows this pattern:

```go
package <pkg>_test

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func Test<PackageName>(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "<PackageName> Suite")
}
```

Packages with database tests (`exports`, `models`) add embedded Postgres setup in `BeforeSuite`/`AfterSuite` within the suite file. See below.

### Package Naming

Test files use the **external test package** convention: `package <pkg>_test`. This enforces testing through the public API of each package.

### Dot Imports

Always dot-import both Ginkgo and Gomega:

```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

## Database Testing with Embedded Postgres

Tests requiring a database use `github.com/fergusstrange/embedded-postgres`. The shared helper `utils.CreateTestDB` starts an embedded Postgres on port 5432, runs migrations, and returns a `*gorm.DB`.

### Setup Pattern (in suite file)

```go
var (
    testDB     *embeddedpostgres.EmbeddedPostgres
    testGormDB *gorm.DB
)

var _ = BeforeSuite(func() {
    cfg := config.Get()
    var err error
    testDB, testGormDB, err = utils.CreateTestDB(*cfg)
    Expect(err).To(BeNil())
})

var _ = AfterSuite(func() {
    err := testDB.Stop()
    Expect(err).To(BeNil())
})
```

### Database Cleanup Between Tests

Each test or `BeforeEach` must clean relevant tables before running. Use raw SQL deletes:

```go
testGormDB.Exec("DELETE FROM export_payloads")
```

This is done in `setupTest` helper functions, not automatically.

## Test Organization (Ginkgo Constructs)

- Use `Describe` for the top-level subject (e.g., `"The public API"`, `"Models"`, `"Db"`).
- Use `Context` to group scenarios (e.g., `"when the source is found"`).
- Use `It` for individual test cases.
- Use `DescribeTable` + `Entry` for data-driven / parameterized tests. This is the **primary pattern** for testing multiple input variations (see middleware tests and exports filter tests).
- Use `BeforeEach` for per-test setup (e.g., creating fresh model instances, cleaning the DB).

## Mock Patterns

### Interface-Based Mocks (No Mock Libraries)

This project does **not** use mockgen, gomock, or any mock generation tool. Mocks are hand-written structs that implement interfaces.

**StorageHandler mock** (`s3/compress.go`): `MockStorageHandler` is defined in the production package (not a test file) and implements the `StorageHandler` interface. It lives at `s3.MockStorageHandler` and is used by test packages.

```go
StorageHandler: &es3.MockStorageHandler{}
```

**Kafka mock**: Kafka message sending is mocked via a function type (`RequestApplicationResources`). Tests pass a closure:

```go
func mockRequestApplicationResources(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
    // no-op
}
```

To test that Kafka was called, use a captured boolean:

```go
var wasKafkaMessageSent bool
mockKafkaCall := func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
    wasKafkaMessageSent = true
}
// ... after serving request:
Expect(wasKafkaMessageSent).To(BeTrue())
```

**Database**: No DB mocking. Tests use the real `models.ExportDB` backed by embedded Postgres via the `DBInterface` interface.

## HTTP Handler Testing

API tests use `net/http/httptest` with `chi.Router`. The pattern is:

1. Build a `chi.Router` with middleware and handler wired up.
2. Create requests with `http.NewRequest` or `httptest.NewRequest`.
3. Add the identity header via a helper: `AddDebugUserIdentity(req)`.
4. Serve with `router.ServeHTTP(rr, req)`.
5. Assert on `rr.Code` and `rr.Body.String()`.

### Identity Header

Tests use a base64-encoded `x-rh-identity` header. A `debugHeader` constant is defined in the test file. Always add it via a helper function, not inline.

### Test Helper Functions

Each test file with HTTP tests defines its own `setupTest` function that:
- Creates handler structs with mocked dependencies.
- Builds a `chi.Router` with required middleware.
- Cleans the database.
- Returns the router.

Data population helpers (e.g., `populateTestData`, `modifyExportCreated`) are defined as package-level functions in the test file.

## Middleware Testing

Middleware tests follow a consistent pattern:

1. Create a minimal `chi.Router` with only the middleware under test.
2. Define an `applicationHandler` that sets `handlerCalled = true`.
3. Assert both the response code and whether the handler was reached:

```go
Expect(rr.Code).To(Equal(expectedStatus))
Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
```

This ensures middleware correctly blocks or passes requests.

## Build Tag Separated Tests

Tests requiring external infrastructure (real Postgres, not embedded) use the `sql` build tag:

```go
//go:build sql
// +build sql
```

These tests are run separately with `go test ./... -tags=sql -count=1`.

## Assertion Conventions

- Prefer `Expect(err).To(BeNil())` for nil error checks (the dominant pattern in the codebase).
- `Expect(err).ShouldNot(HaveOccurred())` and `Expect(err).NotTo(HaveOccurred())` also appear and are acceptable.
- Use `ContainSubstring` for checking response bodies -- do not unmarshal the full response unless you need specific field values.
- Use `Equal` for exact value matching.
- Use `HaveOccurred()` when checking that an error IS expected.

## Key Conventions Summary

| Aspect | Convention |
|---|---|
| Framework | Ginkgo v2 + Gomega, dot-imported |
| Package style | External (`_test` suffix) |
| DB testing | Embedded Postgres via `utils.CreateTestDB` |
| Mock approach | Hand-written interface implementations, no codegen |
| HTTP testing | `httptest.NewRecorder` + `chi.Router` |
| Parameterized tests | `DescribeTable` / `Entry` |
| DB cleanup | Manual `DELETE` in setup helpers |
| Identity | Base64 `x-rh-identity` header constant |
| Build tags | `sql` for tests needing real Postgres |
| Test runner command | `ginkgo -r --race --randomize-all --randomize-suites` |
