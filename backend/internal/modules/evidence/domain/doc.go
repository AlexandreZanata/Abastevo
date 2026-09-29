// Package domain owns the private-evidence upload session (P05-T01):
// reservation of owned upload intent under structural limits, before any
// expensive object-store work. Construction takes server time, generated
// IDs, server-resolved attribution and a server-generated quarantine key
// from the caller; the client contributes only its submission ID, media
// description and hash claim, never contributor identity, trust or object
// keys (B-BR-004/010, acceptance). State follows the API contract machine
// ISSUED→VERIFYING→READY/REJECTED with ISSUED→EXPIRED after 24 h idle.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package).
// Persistence, presigned URLs, media verification and binding land in
// later P05 tasks; nothing here performs I/O.
package domain
