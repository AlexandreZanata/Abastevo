# MEX-14 — Wire private sync transport and constrained worker

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `6d195688` (maintained dev, MEX-31 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-10/
11; owns PRM03 and PRM09-T03 consumer wiring. Rules B-BR-MEX-05/06;
BUC-MEX-04. Risk: critical auth/jobs/privacy.

Before behavior: private ledger commands queue durably on-device and
upload only after explicit opt-in, one command per worker run, with
the live owner proof attached. Stable IDs/revisions/cursors survive
reconnects; lost replies retry with the same operation ID; duplicate
workers lose the atomic claim race; stale revisions park as conflicts
instead of deleting held work; lapsed sessions and disabled sync never
touch the wire. A local save is not a cloud acknowledgement.

## Delivered

- `domain/repository/LedgerSyncGateway.kt`: port plus receipt,
  conflict and denied typed errors; implementations never invent
  revisions or replay locally.
- `data/remote/ledger/LedgerSyncHttpClient.kt`: typed POST to
  backend `/v1/ledger/commands` (family_id + access_token + one
  command; exact operation_id/dataset/id/expected_revision/delete/
  void/payload; no-store, no redirects, no retry-on-connect, 20 s
  call timeout, 1 MB budget, cancellable). 200 validates
  one-row-one-id receipts; 409/400 park as terminal conflicts;
  401/403/404 deny with retry budget; outages/lost replies retry
  idempotently; corrupt/oversize/foreign/mismatched refuse.
- `PremiumCommandStore` + `PremiumOutboxDao` + `PremiumStoresImpl`:
  atomic `claim` (single-winner), `acknowledge` (delete on server
  durability), `failTransient` (bounded exponential backoff
  30 s→1 h, stays dispatchable), `markConflict` (IN_FLIGHT-only,
  never dispatchable again, no data loss).
- `application/usecase/ledger/DispatchLedgerCommandUseCase.kt`:
  enabled-flag gate (opt-in OFF = quiet DONE), live session proof
  required before any I/O (lapse = REFUSED, queue kept), one
  head-of-queue or requested command per call with 5-minute claim
  lease, ack-by-receipt, stale-revision conflicts park, denied/lost
  replies keep bounded retry. Duplicate claim losers RETRY.
- `data/worker/LedgerSyncWorker.kt` + `LedgerSyncScheduler.kt` +
  `LedgerSyncFlagStore.kt` (+ `application.port/
  LedgerSyncFlagProvider`): Hilt worker mapping DONE→success,
  RETRY→retry, REFUSED→failure; unique `ledger_sync` work with
  CONNECTED constraint and 30 min linear backoff (KEEP converges
  duplicates/reconnects); explicit opt-in default OFF in app-private
  prefs (no account data, no server round-trip to read).
- `LedgerSyncModule.kt` + `RepositoryModule`/`UseCaseModule`
  bindings (no duplicate provision; gateway bound to the HTTP
  client).

## Actual checks

From this worktree (2026-10-26, parent `6d195688`):

- RED: dispatch use-case tests failed to compile before each unit;
  a no-arg mockk `eq` gap, `capture(slot)` arity mismatch,
  helper-inside-class placement and mockk `internal`-member access
  were fixed without weakening tests (all six dispatch cases GREEN).
- `DispatchLedgerCommandUseCaseTest`: opt-in OFF no-op (no wire),
  lapsed session REFUSED with queue kept, accept/replay converge to
  one ack, lost reply retries same op id, stale revision parks as
  CONFLICT without delete, duplicate claim loses and retries.
- `LedgerSyncHttpClientTest` (MockWebServer): exact wire shape
  (family_id, single command, no Authorization header, no-store),
  receipt validation, mismatched-200 → denied, 409/400 → conflict,
  503/oversize → IOException, throttled-body cancellation, municipal
  unknown-IBGE refused before wire, unique-IBGE executes.
- `LedgerSyncWorkerTest`: outcome→Result mapping (success/retry/
  failure). Premium fakes extended for the four new store methods.
- Full JVM suites (app 433 + data 518 + domain 672 + application
  415 = 2038 tests, 0 failures, 0 errors; 2 skips are pre-existing
  live-catalog/backend interop, unrelated); `:app:lint` +
  `:data:lint` green; app compile (Hilt graph incl. new bindings)
  green; `git diff --check` clean; secret scan passed; generated
  Room 24.json schema artefact removed per repo practice (DB already
  at v24, no MEX-14 migration).
- No backend/OpenAPI/SQL change beyond the additive IN_FLIGHT-only
  conflict UPDATE (no migration); device/emulator validation
  remains deferred per standing directive; no PII/secrets in Git.

## Limits

- No live server round-trip here: two-device import/update/delete
  and reconnect scheduling run against staging in the integration
  phase; MockWebServer covers the wire contract only.
- Proof freshness/rotation and conflict resolution UI belong to
  MEX-15; master refresh cadence (OPT-A) belongs to MEX-32.

Next: MEX-15 (conflict, devices, backup and restore controls) once
authorized; it consumes the parked CONFLICT rows unchanged.
