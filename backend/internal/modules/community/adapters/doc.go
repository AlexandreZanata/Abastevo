// Package adapters persists community facts and decisions (P04-T03). It
// owns the generated community queries package; no other module may import
// it. Submit writes the observation, reserves idempotency through the
// natural key and enqueues the validation job in a single transaction, so
// accepted writes are never unprocessed and retries converge instead of
// duplicating. Decisions append through the same store with the state
// machine enforced at the boundary: unknown transitions and out-of-order
// sequences fail before any row exists. There is deliberately no update or
// delete path for facts or decisions anywhere in this package.
package adapters
