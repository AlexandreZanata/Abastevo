// Package application owns anonymous official read use-cases (P02-T08):
// current price groups with independent official/community sections and
// revision-aware price history. Community sections stay null until P04; a
// null is UNKNOWN, never an ANP substitution (B-BR-001). Depends on its
// domain, explicitly declared ports and wire vocabularies only.
package application
