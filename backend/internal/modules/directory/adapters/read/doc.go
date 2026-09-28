// Package read implements the directory StationReader port on the owned
// generated queries (P02-T08). It owns the directory queries package; no
// other module may import it. Reads are projection-only: search walks ID
// order, nearby walks distance order through the GiST index with keyset
// cursors, and missing locations stay honest nulls. LIKE patterns escape
// wildcards, and N+1 CNPJ lookups stay bounded by the page limit.
package read
