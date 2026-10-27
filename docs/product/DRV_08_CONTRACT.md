# DRV-08 — Durable shift and distance persistence contract

Date: 2026-10-26. B-BR-DRV-06/07/08/10/17/18; BUC-DRV-02/03/06. Freeze before
behavior.

## Scope and boundaries

This phase adds private PostgreSQL storage for the existing pure shift and
distance rules. It does not add an HTTP route, Premium admission, cloud sync
consent, GPS collector, location permission, raw coordinate field or mobile UI.
The existing earnings adapter and ledger tables are unchanged. Migration 000134
is additive after 000133; applied migrations are never rewritten.

All mutation access remains behind the transaction-bound ownership port owned
by cloudsync. A missing port denies. Signed HTTP authorization, live Premium
capability and dataset consent remain DRV-10–16 obligations.

## Shift state

The server persists one non-CLOSED shift per owner. The identity is the
owner-scoped shift ID, origin, owned vehicle and explicit IANA timezone. The
client never writes a state, version, active duration or receipt directly.

- State machine: READY, RUNNING, PAUSED, CLOSED, RECOVERY_REQUIRED.
- Actions: START, PAUSE, RESUME, END, CHECKPOINT, RECOVER, REVIEW_GAP.
- A new shift begins READY at version 0 in the pure model and is persisted only
  after its first accepted START. Every accepted command increments version by
  one; an exact retry returns the stored receipt and current state.
- Operation IDs, shift IDs and reviewed gap IDs are lowercase UUIDs. Boot IDs
  are 1–64 ASCII letters, digits, dot, underscore or hyphen.
- `expectedVersion` fences stale pause/resume/checkpoint/end commands. A
  conflicting operation body, an obsolete version or an invalid transition is
  an explicit conflict, not a silent overwrite.
- Active milliseconds use monotonic boot/elapsed evidence. UTC timestamps are
  retained for reporting. A boot change, elapsed/wall rollback, wall/monotonic
  divergence beyond the caller policy or an excessive checkpoint gap creates a
  persisted gap and RECOVERY_REQUIRED; missing work is never invented.
- The caller supplies the checkpoint-gap and wall-tolerance policy. No default
  threshold is frozen here.
- Gaps retain the previous and next boot/elapsed/UTC evidence, a bounded reason
  code, an explicit reviewed flag and the owner-reviewed work milliseconds.
  Review moves the shift to PAUSED without restarting GPS; reviewing without a
  work quantity keeps the reviewed flag but leaves complete work time unknown.
- At most 50 gaps and the last 50 command receipts per shift are retained. The
  version fence still applies after an old receipt is pruned.
- A partial unique index permits only one non-CLOSED shift per account. An
  owner-scoped advisory transaction lock serializes commands, so two devices
  cannot both create an active shift.

## Distance observations

Only aggregate observations are stored; no coordinate, raw trace, passenger or
free-text field is admitted.

- Basis: ODOMETER, GPS_ESTIMATE, PAID_REPORTED, MANUAL_TRIP, ROUTE_PREDICTION.
- Scope: owner, owned vehicle, UTC half-open window with IANA report timezone,
  and an opaque vehicle segment label.
- Coverage: integer metres may be unknown; completed coverage requires known
  metres and no gap. A gap or incomplete classification remains explicit.
- Quality: MEASURED, ESTIMATED, REPORTED or UNKNOWN, an optional positive
  policy version and a bounded reason code. Basis/quality combinations are
  fixed: odometer/manual trip are measured, GPS/route are estimated, paid
  reported is reported.
- ODOMETER observations retain the chosen start/end odometer metres. Both are
  required, non-reversed and inside the same segment; the stored distance must
  equal their difference. Route prediction is never a measured denominator.
- The same owner/vehicle/basis cannot have overlapping windows.
  Different bases may share a window for reconciliation and are never added.
- An identical operation/body retry converges; a changed body, a duplicate ID
  with different content or an overlapping observation conflicts. No silent
  clipping, averaging or summing of observations is allowed.

## Account lifecycle and evidence

The new tables cascade on physical account deletion. A soft-delete trigger
erases shifts, gaps, distance observations and their command receipts. Export,
backup, restore and epoch/tombstone registration remain later-phase obligations
and are not claimed by this storage boundary.

Real disposable PostGIS must prove empty and 133→134 migration, concurrent
devices/retries, stale versions, conflicts, rollback and owner/vehicle
isolation. JVM/native/HTTP/UI proof is not produced in this backend-only phase.
