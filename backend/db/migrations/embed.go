// Package migrations embeds the ordered append-only SQL chain applied by
// the migrate command and integration tests. Never edit an applied file.
package migrations

import "embed"

// Files holds db/migrations/*.sql for the runner and tests.
//
//go:embed *.sql
var Files embed.FS
