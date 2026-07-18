// Package migrations embeds the *.sql migration files in this directory so
// the compiled binary carries its own migrations and does not depend on a
// migrations/ directory being present on disk at runtime (e.g. inside the
// Docker image built by Task 2's Dockerfile).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
