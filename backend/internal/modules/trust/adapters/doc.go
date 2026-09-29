// Package adapters persists the conservative trust ledger (P06-T03).
// It owns the generated trust queries package; no other module may
// import it. Decisions append with the current-tier projection in one
// transaction plus the affected-key recompute job, so readers never
// observe a verdict without its projection and no accepted decision
// waits without downstream work. History stays immutable: blocks,
// reversals and rehabilitations arrive as new rows, and unknown
// contributors read as NEW. The recompute consumer lands in P06-T05;
// until then its jobs park for audited replay.
package adapters
