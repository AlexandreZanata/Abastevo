// Package domain owns official revision decisions (P02-T06, B-BR-013):
// identical source bytes never open a duplicate revision, corrected bytes
// open a new one that supersedes, and nothing publishes without passing the
// review gate (non-empty staged set, quarantine share at or below 1%,
// row-count drop at or below 20% against the comparable revision).
// Thresholds come from ANP_INGESTION import stage 6 and are versioned with
// the code that enforces them, not with operator memory.
//
// Boundaries: stdlib only (TestDomainStdlibOnly covers this package too).
// Persistence and the publication pointer live behind the Store port in
// official/adapters; staged rows of failed or under-review revisions stay
// unreachable by construction because only the pointer grants visibility.
package domain
