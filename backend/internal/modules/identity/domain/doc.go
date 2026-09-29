// Package domain owns anonymous identity rules (P03-T02, B-BR-004): challenge
// purposes and expiry, fingerprint shapes, nonce binding format and the
// invariant that server identity always derives from a verified key proof,
// never from a body-supplied ID. Same key always resolves to the same
// contributor; a lost key cannot recover the identity.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package too).
// Proof cryptography lives in the profile package; persistence and the
// registration transaction live behind the Store port in identity/adapters.
package domain
