// Package application assembles contributor exports (P07-T03): one
// durable request per owner operation, a bounded owner-only archive
// built over an injected inventory port, and an owner-gated expiring
// download. The inventory port keeps this package decoupled from
// sibling modules: tests use fakes, and the worker wires real
// identity/community/trust readers. Depends on its domain and ports
// only — never on adapters, SQL or other modules.
package application
