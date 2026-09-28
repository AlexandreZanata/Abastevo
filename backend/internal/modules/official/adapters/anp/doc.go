// Package anp parses ANP workbook streams with bounded resources (P02-T04,
// D06 SELECTED: standard archive/zip plus encoding/xml, zero new
// dependencies). It reads the subset ANP actually emits — stored strings,
// shared strings and numeric cells — and never evaluates workbook formulas:
// cached values are read as text, cells without a cached value stay empty
// for the import flow to quarantine. Layout comes from validated sheet
// names and header signatures, never from fixed row offsets alone (A07).
// Every limit, bomb guard and refusal below is deliberate: a parser that
// silently coerces, skips or evaluates is a corruption vector, not a reader.
package anp
