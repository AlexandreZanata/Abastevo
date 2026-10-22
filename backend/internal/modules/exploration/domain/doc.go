// Package domain interprets supported Portuguese questions into frozen
// typed exploration queries (B-BR-MEX-08–13). Resolution is an allowlisted
// deterministic grammar over normalized text: never executable SQL,
// scripts, remote URLs, an expression language, a search service or a
// language model. Ambiguous intent yields labelled alternatives; anything
// else yields an explicit unsupported reason, never a fabricated answer.
//
// Boundaries: standard library only. No I/O, no SQL, no network, no clock
// beyond the caller-supplied reference date. Unknown words are ignored
// unless they form a supported intent; adversarial text is treated as
// literal words, never evaluated.
package domain
