# SEC04-B — Deletion linkage: intent + revoke + enqueue

Date: 2026-10-11. Status: LOCAL_DONE. Scope: durable intents,
atomic revoke, wired scope cleanup — no training/consent split
(C), no local-queue sweep (D). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC04, AUD-05). Contract: B-BR-A05 (export/erasure), B-BR-016
(ledger/replay). Constraint: bindings captured BEFORE dropping
(the AUD-05 root); receipt names intent, never completion; no
distributed transaction, no new service.

Validation: PASS

## Changes

- Migration `000059_account_erasure_intents` (append-only) +
  sqlc (`InsertErasureIntent` convergent, `ListErasureIntentsByAccount`):
  one row per (account, contributor), written in the
  DeleteAccount transaction.
- `account` MemStore + PGStore: `DeleteAccount` captures bindings,
  writes intents, revokes families and unlinks atomically,
  returning captured IDs (sorted); `ListErasureIntents` replays
  durable intents after crashes.
- `account` Service: `RequestErasure` port (nil refuses closed
  before any mutation with `ErrErasureWiringMissing`);
  `DeleteAccount` returns `DeletionReceipt{AccountID,
  ContributorIDs, ErasureRequested}`; empty-capture retries replay
  from intents (convergent scope cleanup).
- `account` HTTP: `DeleteAccount` returns the receipt
  (`{status: deleted, erasure_requested}` — counts/flags only,
  contributor IDs stay server-side); wiring-missing maps to
  500 without identity echo.
- `cmd/api`: wires `RequestErasure` — one privacy deletion
  intent per contributor (deterministic
  `account-erasure:<account>:<contributor>` submission,
  reason `account-deletion`, durable `privacy-erasure` job
  through the deletion ledger) plus synchronous
  `feedbackService.EraseAccount` (tombstone + rebuild,
  convergent re-runs).
- Tests: 4 unit (capture+enqueue+receipt, wiring-missing
  no-mutation, crash replay, binding-free empty call); HTTP
  receipt assertion; real-PostGIS capture/intents/revoke/replay
  proof; real-PostGIS wired composition (privacy ledger intent
  + tombstoned comment + unreadable view).

## Validation (actual)

- `go test ./internal/modules/account/... ./internal/modules/feedback/... ./internal/modules/privacy/...`
  green. `-race -tags=integration` green: account adapters
  (incl. 2 new SEC04-B; migration 000059 applied on fresh DBs)
  + feedback adapters re-green.
- `go build ./...`, `go vet`, `gofmt -l` clean, `sqlc vet`
  clean, `git diff --check` clean, `scan-secrets.sh` passed
  (synthetic addresses only).
- RED pre-GREEN: pre-fix `DeleteAccount` left comments readable
  with public alias after deletion (audit probe, real SQL);
  nil-port test failed before fail-closed ordering (mutation
  first); composition failed before UUID-safe generators.

## Limits / owed

- Worker executes privacy scopes asynchronously: full
  account→reads→restore evidence rides SEC04-D; intent rows +
  ledger make every crash repairable and every retry convergent.
- Training/consent reconciliation (revoke vs operational expiry):
  SEC04-C. Local queue/media sweep under erased owners: SEC04-D.
- Multi-replica serializability beyond row locks/advisory xact
  not proven. No VPS/phone change; no production claim.

Next microtask: SEC04-C attribution + aggregates + training split.
