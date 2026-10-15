// Package read serves anonymous quality ledger reads. Every method
// resolves the current or explicitly selected historical revision and
// scopes all rows to it, so one call sees one frozen lease across concurrent publications.
// Source CNPJs and internal notes never leave this package.
package read
