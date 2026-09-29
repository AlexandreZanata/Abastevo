// Package jobs is the durable PostgreSQL work queue (P03-T07): no broker,
// no in-memory authority. Producers enqueue inside their business
// transaction; workers claim with SKIP LOCKED under fencing leases, so at
// most one worker holds a job and a stale holder can never complete it.
// Delivery is at-least-once by design: exactly-once business effects come
// from consumers keying idempotent work by the job dedupe key. Poison jobs
// park in DEAD after capped attempts instead of retrying forever, and
// replay is always an explicit audited action. Pause a kind by stopping its
// workers; fix the handler; replay DEAD rows with a reason.
package jobs
