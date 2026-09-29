// Package application opens moderation cases and serves the operator
// queue (P07-T01): one bounded case per review target with a priority,
// a mandatory reason and an optional safe evidence reference, composed
// through explicitly declared ports. Opening is worker-driven (dispute
// reports, trust abuse signals, evidence mismatches); operator
// authentication and audited actions land in P07-T02, so this package
// authenticates nobody itself. Depends on its domain and ports only —
// never on adapters, SQL or other modules.
package application
