// Package adapters implements the identity Registrar port on pgx (P03-T02).
// It owns the generated identity queries package; no other module may
// import it. Challenges are bound to one fingerprint and purpose with a
// stored nonce hash; registration verifies the key proof first, then
// consumes the challenge and creates the contributor plus key in a single
// transaction. A failed proof never consumes, so clients can retry; a valid
// proof for an already-registered fingerprint returns the existing
// contributor instead of forking identity. UUIDs and nonces come from
// crypto/rand; private keys never reach the server.
package adapters
