//go:build tools

package tools

// Pin transitive dependency versions for security fixes.
// golang-migrate/migrate declares mongo-driver v1.7.5 which has CVE-2026-2303.
import _ "go.mongodb.org/mongo-driver/mongo"
