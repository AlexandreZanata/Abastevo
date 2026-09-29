// Package application owns community write and owner-read use-cases
// (P04-T04): observation submit with safe acknowledgment, owner-only status
// and owner history. Submit composes quota, idempotency, validation and
// persistence through explicitly declared ports; pgx.Tx appears only as an
// opaque handle threaded into those ports, never queried here, because the
// atomic submit spans the observation insert and the job enqueue in one
// transaction. Depends on its domain and ports only — never on adapters,
// SQL or other modules.
package application
