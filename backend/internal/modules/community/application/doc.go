// Package application owns community write, owner-read and validation
// use-cases (P04-T04, P04-T05): observation submit with safe
// acknowledgment, owner-only status and owner history, plus validation
// orchestration that claims received observations with the persisted job
// ID and persists terminal transitions with the consensus intent. Submit
// and Validate compose quota, idempotency, read-only station/trust/
// evidence ports and persistence through explicitly declared ports;
// pgx.Tx appears only as an opaque handle threaded into those ports,
// never queried here, because atomic steps span a decision insert and a
// job enqueue in one transaction. Depends on its domain and ports only —
// never on adapters, SQL or other modules.
package application
