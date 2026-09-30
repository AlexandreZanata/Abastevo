// Package domain freezes FREE account/session/provider/recovery semantics
// (P13-T01, B-BR-A01…A05): OTP bounds, session lifetimes, OIDC verification
// rules and the exact subject/session/device proof boundary for social
// writes. Attack fixtures in contracts/testdata/identity/account-*.json pin
// the adversarial cases; handlers (P13-T02/T03), recovery (P13-T04) and
// native login (P13-T05) consume these constants, ports and verdict codes.
//
// Boundaries, per TARGET_ARCHITECTURE: stdlib only (no I/O, no network, no
// float money); persistence, mail delivery and provider JWKS live behind the
// Store/Mail/Verifier ports implemented in later tasks.
package domain
