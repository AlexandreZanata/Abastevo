# SEC04-D — Replay proofs and erased-owner local cleanup

Date: 2026-10-11. Status: LOCAL_DONE. Scope: ledger replay/parallel
proofs (backend) + owner queue/media purge with send-guard wiring
(Android) — no worker-scope changes. Head: maintained `dev` + this
task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC04, AUD-05). Contract: B-BR-016 (ledger/replay), B-BR-A05;
Android outbox owner scope (payload-embedded, no new columns).
Constraint: unattributable rows are never deleted (fail-closed);
transport failure keeps the account, the backup AND the queue; no
PII in logs/tests (synthetic scopes only).

Validation: PASS

## Changes (backend)

- `privacy/application/erase_replay_test.go` (new): lost-response
  duplicate intents converge on the recorded request; partial
  restores replay exactly the surviving scopes and stamp them
  (others untouched, nothing invented).
- `account/application/deletion_erasure_test.go`: 8 parallel
  duplicate deletions converge — single intent capture, every
  caller receipted, convergent downstream enqueue.

## Changes (Android)

- `domain/.../ContributionOutboxPorts.kt`: `purgeOwned(ownerScope)`
  (fail-closed on blank/unparseable scopes).
- `data/.../RoomContributionOutboxRepository.kt`: implements it
  (scope parse shared with `listOwned`, `deleteById` + scheduler
  cancel per row, purged count); no schema change (scope lives in
  the payload, quarantined-from-dispatch rule preserved).
- `data/.../PhotoUploadSessionDao.kt`: `listByOwner` +
  `deleteByOwner` (column already existed).
- `data/.../OwnerLocalErasure.kt` (new): purges outbox rows,
  sealed-cache copies (photo+capture ids, best-effort) and session
  rows for the current/captured scope; identity-unavailable and
  storage failures degrade to zeros, never throw.
- Send guard (pre-existing, now load-bearing): gateway
  `checkScope` refuses mismatched-owner sends with
  `owner-or-origin-changed` (DomainException → REFUSED, never
  sent); identity-unavailable fails before any send. Crash
  residuals between logout-clear and purge can never send under a
  new owner.
- `AuthViewModel`: logout purges before identity clears (local
  always clears); deletion captures scope first and purges only
  when local authority is gone (success/denial), never on
  transport failure.
- Tests: Room purge isolation (scoped/unscoped/unparseable/other
  owner), erasure orchestration (counts, isolation, no-identity
  no-op), ViewModel logout/delete/failed-delete wiring, fakes
  updated for the new port.

## Validation (actual)

- Backend: account/privacy unit green (incl. 3 new replay
  proofs); prior real-PostGIS SEC04-B/C proofs untouched.
- Android: `:domain:test`, `:application:test`,
  `:data:testDebugUnitTest` (full), `:app:testDebugUnitTest`
  (full) green — incl. 7 new tests (1 Room, 3 erasure, 3
  ViewModel).
- `git diff --check` clean; no secrets/PII in fixtures.
- RED pre-GREEN: ViewModel/ Room tests failed before wiring
  (no purge path existed — the AUD-05 local gap); replay tests
  failed before stamp-aware assertions (raw ledger rows never
  mutate).

## Limits / owed

- Full account→device→restore evidence with a real worker run
  from an account deletion (privacy scopes executing off the
  durable intents) is an async staging proof, not claimed here;
  every crash is repairable via intents/ledger and every retry
  converges (proven).
- Sealed-cache copies without a session row (orphan `.tmp`,
  SEC05-C/AUD-07 lane) ride the Android transient-cache cleanup,
  not owner purge. Process-death between purge steps may leave a
  subset; the next logout purges again and the send guard holds.
- No VPS/phone change beyond local unit scope; no production
  claim.

SEC04 exit complete (A/B/C/D). Next microtask: SEC05-A overdue inventory.
