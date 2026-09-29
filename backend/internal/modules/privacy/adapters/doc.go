// Package adapters persists the privacy request ledger (P07-T03).
// It owns the generated privacy queries package; no other module may
// import it. Requests converge retries on the owner natural key, move
// forward only through guarded transitions, and read back owner-scoped
// so one contributor can never observe another's request. Archives are
// bounded owner-inventory bytes with a recorded hash; expiry purges
// land with the retention scheduler (P07-T05).
package adapters
