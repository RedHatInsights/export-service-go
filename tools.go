//go:build tools

package tools

// Pin transitive dependency versions for security fixes.
// golang-migrate/migrate declares mongo-driver v1.7.5 which has CVE-2026-2303.
import _ "go.mongodb.org/mongo-driver/mongo"

// golang-migrate/migrate declares grpc-gateway/v2 v2.27.1 which has CVE-2026-37236.
import _ "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
