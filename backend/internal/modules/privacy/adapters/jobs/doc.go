// Package jobs runs the durable privacy workers (P07-T03): the export
// build consumer that assembles one bounded owner archive per request.
// Exactly one request travels per job; terminal requests replay as
// no-ops, failures retry with backoff until the cap, then park for
// audited replay. Payloads carry identifiers only, never owner data.
package jobs
