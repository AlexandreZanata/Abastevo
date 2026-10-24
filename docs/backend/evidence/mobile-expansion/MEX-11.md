# MEX-11 — Durable Android ledger, drafts and command outbox

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `f7d8110d` (maintained dev, MEX-08 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-08/
10, accepted (PRM09-T01/T05). Rules B-BR-MEX-03/05/06, BUC-MEX-02/04.
Risk: private persistence.

Before behavior: private ledger entries persist as validated drafts
and dispatchable commands with exact bodies and expected revisions.
Double-saves converge, lost acknowledgements retry with the same
operation ID (the backend replays instead of duplicating), crashed
claims release on restart, and account/origin switches never read or
resume another owner's queue. Recovery: stop affected writes, preserve
existing rows and pending work.

## Delivered

- `domain/premium/LedgerDraft.kt`: validated refuel/expense/charge
  drafts with backend-parity bounds (fuel/unit pairing, null omission,
  tariff atomicity, SOC range, note hygiene, charge odometer ceiling)
  plus exact canonical bodies and `LedgerCommandFactory` (deterministic
  operation IDs from owner/dataset/record/revision/body). Added the
  `CHARGE` ledger dataset.
- `ledger_draft` table + DAO (partition-keyed, REPLACE on stable draft
  id), migration 21→22 (additive), database v22 + Hilt provisions;
  `LedgerDraftStore` port + Room impl with strict body codec
  (round-trip; corrupt/drifted rows fail closed).
- Reused `PremiumCommandStore` for dispatch (divergent-reuse refusal
  kept) plus `releaseExpired` port/DAO for restart recovery.
- `SaveLedgerDraftUseCase` / `EnqueueLedgerCommandUseCase` /
  `RecoverLedgerOutboxUseCase` (+ Hilt). No transport here; MEX-14
  owns dispatch.

## Actual checks

From this worktree (2026-10-26, parent `f7d8110d`):

- RED: domain/data/application tests failed to compile before each
  unit; a control-char note gap vs the backend, a CNG-validity
  correction, runBlocking-in-test and a lost-newline merge were fixed
  without weakening tests (parity decisions documented in-code).
- Domain goldens (refuel/expense/charge bodies byte-exact, nulls out),
  validation table, op-ID stability/divergence; data migration
  SQL-capture guard, draft DAO fake (CRUD/partition isolation),
  codec round-trip + corrupt/drifted/foreign refusal, repo mapping;
  application double-save convergence, idempotent enqueue, correction
  new-ID, invalid/missing refusal, guest-vs-account scoping, recovery
  passthrough.
- Full `:domain:test :data:testDebugUnitTest :application:test
  :app:testDebugUnitTest --offline` → 1967 tests, 0 failures;
  `:app:lintDebug :data:lintDebug` green; `git diff --check` + secret
  scan clean. No backend/OpenAPI change; no PII/secrets in Git/logs.
  Device reopen stays deferred per standing directive.

## Limits

- No HTTP dispatch or sync accounts yet (MEX-14 owns transport; guest
  `local` owner until then). Draft decode of future datasets refuses
  (fail closed, no silent upgrade).
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-12 (refuel/expense journeys) once authorized; it consumes
these drafts and commands unchanged.
