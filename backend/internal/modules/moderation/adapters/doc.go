// Package adapters persists the moderation case queue (P07-T01).
// It owns the generated moderation queries package; no other module may
// import it. Cases open once per target: duplicate reports converge on
// the open case instead of flooding the queue, and the queue lists
// actionable cases by priority then age with a keyset cursor. Rows carry
// identifiers, priority, reason and an optional evidence reference only:
// no coordinates, media bytes, IPs or URLs ever land here (B-BR-011).
// Status moves through audited actions in P07-T02; no UPDATE or DELETE
// path exists in this package.
package adapters
