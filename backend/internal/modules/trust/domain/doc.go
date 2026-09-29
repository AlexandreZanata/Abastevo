// Package domain owns the conservative trust ledger (P06-T03):
// immutable trust decisions, a rebuildable current-tier view and the
// promotion policy. Reliability promotes only from independently
// reviewed outcomes (moderator review or validated pilot labels), never
// from raw submission volume, consensus success or any paid plan —
// payment fields do not exist in this package by construction. BLOCKED
// is an audited abuse decision with case references; rehabilitation
// arrives as a new decision, never an edit.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package).
// Persistence and operator wiring land in later tasks; nothing here
// performs I/O.
package domain
