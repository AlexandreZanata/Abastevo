# MEX-12 — Vehicle refuel/expense Android journeys

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `7f4b300` (maintained dev, MEX-11 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-11,
owns PRM09-T05. Rules B-BR-MEX-02–05/18, BUC-MEX-01/02. Risk: money
input/lifecycle.

Before behavior: per-vehicle refuel/expense entries are created,
corrected and deleted as durable drafts with full/partial/unknown
fills, optional odometer/station, expense categories and localized
pt-BR parsing. Drafts queue as idempotent commands with truthful
LOCAL/PENDING/CONFLICT states; gaps and segment resets queue as held
intents; an optional estimated km/L is configured per vehicle. Rapid
saves converge via double-tap guard, process death restores drafts,
offline work loses nothing, and paid sync stays inaccessible without
data loss. Recovery: retain readable/exportable entries; scoped UI
rollback only.

## Delivered

- `LedgerEntriesViewModel/Screen` + `vehicle-entries/{id}` route with
  per-row entries button: list with state chips, add/edit form (dataset
  chips, date/time, quantity/total/unit-price with comma decimals,
  fill/category chips, optional odo/station/note, typed per-field
  errors), queue/delete with confirm, gap dialog (after + optional
  before/note), reset confirm, estimate section. BEV hides refuel with
  an explicit reason; double-tap save is ignored (no forked drafts).
- `ListLedgerEntriesUseCase` (drafts × queued commands → LOCAL/PENDING/
  CONFLICT; conflict = correction drafted after enqueue),
  `GetLedgerEntryDraftUseCase`, `DeleteLedgerEntryUseCase` (draft
  delete + convergent tombstone), `DeclareLedgerGapUseCase` /
  `ResetLedgerSegmentUseCase` (stable intent op IDs),
  `EstimatedEfficiencyUseCase` + SharedPreferences store (validated
  km/L, local-only, clearable).
- Domain: gap/reset intents with exact bodies, `EstimatedEfficiency`
  (0,1000] km/L, ≤2 decimals), stable intent command factory
  (`ledger-gap|reset`), LPG refusal (no ledger unit — no aliasing),
  `deleteCommand` tombstones.
- 50 new strings in all 8 locales; no HTTP (transport stays MEX-14);
  backend OpenAPI/SQL untouched.

## Actual checks

From this worktree (2026-10-26, parent `7f4b300`):

- RED: intent/use-case/VM tests failed to compile before each unit; a
  silent blank-vs-invalid abort (parse helpers now distinguish absent
  from malformed), a property/function name collision that swallowed
  calls, an invalid-UUID fixture, double KSP bindings, and duplicate
  nav args were fixed without weakening tests (a throwing-stub probe
  and `showForm` tracing isolated the silent path).
- Domain intent/estimate tests; data pref-store tests; application
  entry/gap/reset/estimate flows (LOCAL/PENDING/CONFLICT, tombstone
  convergence, note validation, restart reload, account scoping);
  VM tests (typed invalid input, double-tap convergence, queue/delete/
  estimate/restart, gap-reset intents with real UUIDs).
- Full `:domain:test :data:testDebugUnitTest :application:test
  :app:testDebugUnitTest --offline` → 1979 tests, 0 failures;
  `:app:lintDebug :data:lintDebug` green (8 locales);
  `git diff --check` + secret scan clean. No backend/OpenAPI/SQL
  change. Device reopen stays deferred per standing directive.

## Limits

- Charge entry UI arrives with EE-20 (domain/VM support already
  present but unwired and untested at the surface). Paid dispatch is
  unwired (MEX-14 owns transport); queued intents are held, visible
  and deletable — never silently dropped or auto-sent.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: EE-20 (private charging entry) once authorized; it consumes
these drafts/commands unchanged.
