// Package adapters owns the Postgres Store for FREE accounts (P13-T02B).
// Every write funnels through the generated sqlc account package; the two
// atomic transitions (code consume, family rotation) run as single
// transactions with row locks plus compare-and-swap, mirroring the memory
// store line-for-line. Timestamps cross the boundary as unix seconds in the
// domain and TIMESTAMPTZ in SQL; UUID helpers are local so no module
// imports another module's adapter. Plaintext codes and tokens never reach
// these statements: only salted hashes persist.
package adapters
