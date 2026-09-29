// Package domain owns the moderation case queue (P07-T01):
// bounded actionable cases from reports and signals (BUC-006, B-BR-012).
// A case names one review target (observation, dispute, contributor or
// evidence) with a priority and a mandatory reason; it never copies raw
// media, exact GPS or network metadata. Status moves OPEN→IN_REVIEW→
// RESOLVED/REJECTED through audited operator actions (P07-T02); this
// package only opens cases in OPEN and ranks them for operators.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package).
// Persistence and operator wiring land in adapters/application; nothing
// here performs I/O.
package domain
