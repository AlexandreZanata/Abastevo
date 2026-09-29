// Package adapters persists evidence sessions and verified objects
// (P05-T04). It owns the generated evidence queries package; no other
// module may import it. Reserve converges retries on the contributor
// natural key and conflicts on divergent payloads; the completion claim
// and the verify-job enqueue commit atomically, as do the READY
// transition and its object row, so accepted work is never unprocessed
// and retries converge instead of duplicating. Object binding claims at
// most one observation with set-if-unbound-or-same semantics. Objects
// stay insert-only; a failed READY commit leaves an unreferenced final
// object for the T05 orphan sweeper. Community modules read through the
// narrow ports in community.go, never through SQL.
package adapters
