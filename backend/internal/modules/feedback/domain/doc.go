// Package domain owns FREE station/fuel feedback values (P14-T01,
// B-BR-F01…F08): integer ratings, 280-scalar plain-text comments,
// integer-basis-point agreement and versioned revision rules. Standard
// library only, mirroring the account and kernel packages: no I/O,
// no float money, no silent truncation. The server enforces every
// bound; clients only hint.
package domain
