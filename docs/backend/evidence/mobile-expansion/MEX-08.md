# MEX-08 — Enforce local free/Premium vehicle admission

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `a3b4c597` (maintained dev, EE-19 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-07,
EE-19, accepted (typed PHEV capability contract). Rules B-BR-MEX-01/
02/05, BUC-MEX-01. Risk: persistence/access/races.

Before behavior: one new local vehicle is free (guest/offline); a
second or third needs the `vehicles.multiple` capability via a valid
versioned owner/origin lease or fresh server proof. Edits and replays
of held IDs never consume a slot; hosted creation never accepts a
local lease; the technical ceiling stays three; legacy over-ceiling
rows stay readable and editable. Recovery: preserve IDs/history and
basic free use; no destructive wipe.

## Delivered

- `domain/premium/VehicleAdmission.kt`: closed admission vocabulary
  (`Scope/Authority/Outcome/Refused/Result`), pure `decide` (free ≤1,
  capability gate ≤3, ceiling above, LOCAL-lease/server-proof
  acceptance, hosted never on local proof), pure lease resolution
  (version/capability/owner/origin match, revocation, rollback clock,
  min(verified+24h, paidThrough) end-exclusive, malformed window) and
  fresh-server-proof authority (900 s, never from the future).
- `VehicleAdmissionRepository` port + `VehicleAdmissionRepositoryImpl`
  (lease partition store, atomic delegate).
- `VehicleDao.admit` (`@Transaction`: id exists → replay; recount →
  ceiling/capability refusal → insert; no bare count-then-save),
  `VehicleCapabilityLease{Entity,Dao}`, migration 20→21 (additive lease
  table), database v21 + DAO/Hilt provisions.
- `SaveVehicleUseCase` gated (guest → `local` owner; lease resolution;
  rule decision; atomic admit backstop; events unchanged);
  `GetVehicleAdmissionStatusUseCase` for honest UI pre-check (no write).
  Default server proof is stale — no fake grant; MEX-14 wires live
  proofs.
- App: cached admission status drives `canAddVehicle`/honest Premium
  entry dialog (data preserved, dismissible); race refusal on save
  surfaces the same entry; ceiling stays silent. Three new strings in
  all 8 locales.

## Actual checks

From this worktree (2026-10-26, parent `a3b4c597`):

- RED: admission/domain/DAO/migration/VM tests failed to compile
  before each unit; a substring `vehicle` false-positive, a missing
  Hilt DAO provision, lost `()` typos and an async pre-check race were
  fixed without weakening tests (pre-check now uses the cached load
  status; save still enforces atomically).
- `VehicleAdmissionTest`: all 23 `vehicle-registration-v2.json`
  vectors decide exactly (free/premium/ceiling, legacy/over-ceiling,
  restore/replay, concurrent, import-atomic, offline valid/expired,
  clock-rollback, hosted refusal, stale proof, unknown policy) plus
  lease-resolution refusals (forged version, stranger owner/origin,
  revoked, rollback, both expiry bounds, malformed window).
- DAO fake-store tests: free→premium→ceiling, edit replay over
  ceiling, serialized racers (recount refuses the second, nothing
  written); migration SQL-capture guard (lease table only, vehicle and
  13 other families untouched); application save/status tests (free,
  edit, premium refusal, ceiling refusal, fresh-proof admission);
  VM tests (honest entry, dismiss, race-refusal entry, legacy edit).
- Full `:domain:test :data:testDebugUnitTest :application:test
  :app:testDebugUnitTest --offline` → 1952 tests, 0 failures;
  `:app:lintDebug :data:lintDebug --offline` green (8 locales);
  `git diff --check` + secret scan clean; no backend/OpenAPI/SQL
  change beyond the additive migration → vacuum/sqlc N/A. Device
  reopen stays deferred per standing directive. No PII/secrets in Git.

## Limits

- No live server proof is wired (default stale): second vehicles stay
  unavailable until MEX-14 sync verification; the lease write path
  exists for tests and future sync only. No purchase flow is added
  (sales stay disabled per premium policy).
- Vehicles remain a device-local partition until MEX-14 sync
  partitions; leases are already owner/origin keyed and never merged.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-11 (durable ledger drafts/outbox) once authorized; it
consumes this admission gate unchanged.
