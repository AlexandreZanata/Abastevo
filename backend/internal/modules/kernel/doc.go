// Package kernel is the minimal shared kernel for stable price primitives
// (P02-T02): fuel products, units, exact milli-BRL prices, sale conditions
// and CNPJ identifiers. It centralizes the import/community invariants from
// B-BR-002 so directory (stations, P02-T03) and community (observations)
// modules share one exact implementation instead of drifting apart.
//
// Boundaries, per TARGET_ARCHITECTURE: domain only depends on the standard
// library (enforced by TestDomainStdlibOnly); no I/O, no goroutines, no
// float money, no database. Quarantine mapping stays a pure function
// (QuarantineCode) so later import stages reuse the same codes the P02-T01
// fixtures record. Product vocabulary follows the OpenAPI wire contract:
// GASOLINE_ADDITIVED on the wire, never legacy GASOLINE_PREMIUM (A05).
package kernel
