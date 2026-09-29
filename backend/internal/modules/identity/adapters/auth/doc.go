// Package auth verifies signed HTTP requests against the frozen profile
// (P03-T03, B-BR-004). It rebuilds the covered base from the live request
// — method, configured canonical authority, path, query and body — verifies
// the proof against the server-stored key, then consumes the nonce
// atomically: concurrent replays converge on exactly one acceptance, and a
// failed proof never burns the challenge. Contributor identity always
// derives from the verified key; blocked contributors and revoked keys
// cannot act. No RAM-only authority exists anywhere on this path.
package auth
