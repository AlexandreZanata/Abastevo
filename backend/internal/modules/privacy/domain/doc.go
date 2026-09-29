// Package domain owns the privacy request ledger (P07-T03/T04):
// contributor-scoped export and erasure intents with bounded,
// expiring outcomes (BUC-007, B-BR-011/016). A request names one owner
// (contributor ID plus its attribution token) and one client operation
// identity; retries converge, divergent payloads conflict. READY
// archives are bounded bytes with a recorded hash and a 24 h download
// window; expiry is derived at read time, never a silent extension.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package).
// Persistence and inventory wiring land in adapters/application;
// nothing here performs I/O.
package domain
