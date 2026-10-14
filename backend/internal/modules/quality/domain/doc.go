// Package domain freezes the FQ-01 fuel-quality batch contracts: adopted
// product rules, source-version mapping shapes, semantic identifiers,
// sample/assay grouping, fuel vocabulary, ownership and manifest schemas
// (B-BR-FQ-01–16, BUC-FQ-01–09). Numeric score policy stays with FQ-12;
// persistence, discovery and reads belong to FQ-04/FQ-05/FQ-08.
//
// Boundaries: standard library only (TestDomainStdlibOnly covers this
// package). No I/O, no SQL, no network. The wire fuel vocabulary reuses the
// kernel Product values by exact string parity (pinned in test); unknown
// source labels stay explicitly unmapped, never coerced. Station identity
// remains owned by Directory: this package binds exact validated CNPJs and
// quarantines the rest for FQ-03 review, never creating stations.
package domain
