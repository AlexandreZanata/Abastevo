// Package jobs connects durable handlers for quality backfill and
// incremental discovery (FQ-05). The discovery handler plans one bounded
// chunk from the observed inventory against the ledger and enqueues one
// import job per new or corrected edition with a converging dedupe key;
// the import handler fetches, prepares and publishes a single edition
// through the shared publisher, then advances the watermark. Both run
// behind the platform dispatcher with payload versions; unknown versions
// fail toward DEAD instead of running blind. No handler publishes
// partial editions, and failed runs leave watermark and publication
// untouched.
package jobs
