// Package migrations embeds the ordered append-only SQL chain applied by
// the migrate command and integration tests. Never edit an applied file.
package migrations

import "embed"

// Files holds the append-only SQL chain and the declared compatibility floor.
//
//go:embed *.sql schema_min_supported
var Files embed.FS
