// Package adapters implements the directory Store port on pgx/PostGIS
// (P02-T03). It owns the generated directory queries package; no other
// module may import it. Concurrent ResolveCNPJ calls converge through the
// partial unique index on active identifiers: the conflict loser re-reads
// instead of creating a duplicate station. UUIDs are version 4 generated
// from crypto/rand; no outside identifier is ever trusted as a primary key.
package adapters
