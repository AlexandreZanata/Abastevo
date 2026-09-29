// Package domain owns the immutable observation aggregate (P04-T01):
// facts captured without overwriting any current price. Construction takes
// server time, generated IDs and resolved attribution from the caller; the
// client contributes only submission content, never contributor identity,
// trust, confidence or timestamps (B-BR-004, acceptance). Corrections append
// new facts with supersedes links (B-BR-003); nothing here mutates.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package too).
// Wire vocabulary mirrors the kernel and OpenAPI enums; a cross-check test
// fails on drift instead of letting the lists diverge silently. Persistence
// and the validation state machine land in later P04 tasks.
package domain
