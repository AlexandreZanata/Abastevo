// Package adapters persists the quality assay ledger behind the
// application Publisher port. All writes run in one transaction per
// publication; readers only see published revisions through the current
// pointer. A partial unique index keeps one published revision per
// edition, so concurrent publishers serialize and replays converge.
package adapters
