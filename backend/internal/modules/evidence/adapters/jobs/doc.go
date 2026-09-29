// Package jobs connects the durable verification handler (P05-T04):
// each verify-evidence job runs one VERIFYING session through the
// application orchestration exactly once and persists the terminal
// transition with its object row. Terminal replays converge, unknown
// payloads fail toward DEAD, and transient transport failures retry with
// backoff until the cap, then park for audited replay.
package jobs
