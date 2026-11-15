# EMP-FLEET-007 — Reservations, blocks and derived availability

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Second commit on `codex/enterprise-f4-fleet007` (based on `origin/dev`
`8c2ee654`), after the AUTH catalog amendment `org-permissions-v5`
([evidence](f4-auth-catalog-v5.md)) that grants `fleet.reservation.manage`.
Real dependencies used: FLEET-002 assets, FLEET-003 lifecycle (deactivation,
disposal), FLEET-005 allocation (structural scope), AUTH-006 structural grants,
FLEET-006 assignments and the assigned-asset port, the platform job queue.
Nothing is faked for them; trips and maintenance holds do not exist and are
answered "no source" explicitly (EDR-194).

Read: task block `ep05-fleet-meters.md` EMP-FLEET-007; `modules/fleet.md` §3,
§4 (BR-FLEET-014, 025..032), §5 (UC-FLEET-06/07), §6.1, §6.2, §6.4, §7, §8, §9,
§10.1, §10.5, §11, §12; `08-state-model.md` §3–§4; `10-offline-sync.md` §10.2;
`07-permissions.md` (v5); EDR-144, 156, 164, 165, 170; evidence of FLEET-003,
FLEET-005, FLEET-006, DOC-014 and RPT-001 (job precedents).

IDs used: B-BR-EMP-294..301, BUC-EMP-148..151, EDR-192..195 (all of them; no
pre-allocated ID is left unused). Migration `000217_*` only; every test derives
the version from one file-name constant (`reservationMigration`) and builds the
previous chain as `name < thisMigration`. The asset-use source is redefined
from its latest body (000215, FUEL-002), keeping every allow-list line.

## Rules frozen before behaviour

Observed before any change: no reservation, block or availability storage or
route exists; `fleet.reservation.manage` was unassigned until the v5 commit;
FLEET-003 deactivation/disposal changes `org_assets.state` and FLEET-006 ends
assignments there through an `AFTER UPDATE OF state` trigger; the job queue is
`job_queue` (`jobs.Enqueue` in the caller's transaction, `not_before`,
de-duplication key). No organization setting holds a no-show tolerance; no trip,
mission or maintenance hold exists.

Rules B-BR-EMP-294..301 and use cases BUC-EMP-148..151 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-FLEET-007 — Reservations, blocks and derived availability"; decisions
EDR-192..195 in [16-decisions](../16-decisions.md).

## Shape

- Pure domain `fleet/domain/reservation.go` (closed states, §6.4 transition
  table with no FULFIL row, tolerance bounds, half-open window, `StateAt`,
  `DueAt`, `NoShowDeadline`, `HoldsAt`, block reasons, `ValidNewBlock`,
  `RemoveBlockAt`) and `availability.go` (§6.2 precedence `Assess` listing every
  cause, `DecideStart` with `ASSET_BUSY`/`ASSET_UNAVAILABLE`).
- Application `fleet/application/reservations.go`: `ReservePolicy` (fresh
  `fleet.reservation.manage`, configuration write), `FleetReserve`, exact
  commands with scoped digests, `ReservationConflict` (window only),
  `ReservationService` (live decision of the exact policy and asset before and
  after the repository, full result/page validation), ports `ActiveUsePort`,
  `MaintenanceHoldPort`, `StartAvailabilityPort`, `ReservationOutcomes`.
- Migration `000217_organization_asset_reservations.sql` (EDR-192): see the
  B-BR rules; guards, exclusion, left-service trigger, usage allow-list.
- Store `fleet/adapters/reservations.go` (+ sqlc `db/queries/fleet/reservations.sql`):
  authority → root lock → receipt replay → one audit event → asset found →
  transaction instant → settle due outcomes → window pre-check (window only) →
  effect → queue outcome job → receipt → authority. `NoActiveUse` and
  `NoMaintenanceHolds` adapters; reads at one database instant; the start port
  on the borrowed intake transaction.
- Job `fleet/adapters/reservation_jobs.go`: kind `org-asset-reservation-outcome`,
  exact v1 payload, per-outcome de-duplication, compare-and-set settlement;
  composed by `enterprisesuite.FleetHandlers` and `cmd/worker` (general
  dispatcher).
- HTTP `fleet/adapters/http/reservations.go`: eight routes (EDR-195); strict
  bodies; `RESERVATION_CONFLICT` (409) registered in `orgapi` with two
  `conflict_window` details carrying `instant` (new optional
  `httpapi.Detail.Instant`); ended → `CONFLICT`; missing removal reason → 422.
- Authorization: `FleetAuthority` admits `FleetReserve`; the company authority
  admits `ReservePolicy`; `ResourceStructure` resolves asset structure for
  `fleet.reservation.manage`; `FleetRecords` accepts the policy.
- Contract: 8 paths, 12 schemas, `RESERVATION_CONFLICT` and the `instant`
  detail in `contracts/openapi/v1.yaml`; inventory `authorization-routes-v1.json`:
  `fleet.reservation.manage` ROUTED (5 routes), `fleet.asset.read` +3 routes
  (routed 21→22, not routed 63→62, normal 110→118, special 24 unchanged);
  router test paths +8; authorization matrix +1 representative
  (`fleet.reservation.manage` on the second-unit asset: 210→220 cells,
  81→85 allowed, 129→135 denied; private-state snapshot includes the three
  new tables); AUTH-011 auditor walk 118 routes (27 positive incl. the three new
  reads, 91 FORBIDDEN).
- Fixtures: `orgtest.GrantReservationReductions` grants the exact consumer
  columns when 217 is installed (every deactivation/disposal producer runs the
  trigger), as `GrantAssignmentReductions` did for 210.

## RED

Logs in `/data/dev/worktrees/f4-logs/fleet007-*`.

- Domain (compiled zero-value stubs) `go test -json ./internal/modules/fleet/domain/`
  → `fleet007-domain-red.jsonl`: 7 new top-level tests failed, 24 top-level /
  163 subtests passed.
- Application (stubs) → `fleet007-app-red.jsonl`: 6 new top-level failed,
  48 / 119 passed.
- Router/contract: routes wired, spec and inventory at `HEAD` →
  `fleet007-cmd-red.jsonl`: 3 failed (inventory unclassified route, company
  composition, full router "undocumented organization route");
  `TestFleetReservation*` in apicontract against the `HEAD` spec →
  `fleet007-contract-red.jsonl`: 1 top-level failed with 12 failing subtests.
- PostGIS schema RED (migration file moved out) `go test -json -race -tags=integration ./internal/modules/organization/adapters -run '^TestFleetReservation|^TestFleetAvailability'`
  → `fleet007-db-red-schema.jsonl`: 8/8 failed (42P01; the upgrade test on the
  missing migration file).
- PostGIS behaviour RED (migration present, store entry points stubbed to zero
  values) → `fleet007-db-red-behaviour.jsonl`: 8/8 failed (the upgrade test
  after the ordinary migrator applied 000217, at the first behaviour check).

## GREEN

- Domain and application: `go test -race` ok (domain 7 new tests, application
  6 new tests). One pre-existing probabilistic failure of
  `TestPrivateValuesRedactEveryFormattingAndSerializationSurface` (a random
  synthetic identifier's `%x` rendering can contain a checked fragment) was
  seen once in the domain package and passed in 7 reruns; it is unrelated to
  this task (no change touches the asset formatting surface).
- PostGIS focused (`-race -tags=integration`, real ordinary NOSUPERUSER /
  NOBYPASSRLS runtime role): 8/8 top-level passed —
  `TestFleetReservationCommandsConflictsAndReplay`,
  `TestFleetReservationReferencesAndAssetLeavingService`,
  `TestFleetReservationTimeOutcomesAtReadTimeAndByJob`,
  `TestFleetReservationConcurrentConfirmationsAndCancel`,
  `TestFleetAvailabilityReadAndStartCheckPort`,
  `TestFleetReservationSchemaGuardsAndIsolation`,
  `TestFleetReservationPopulatedUpgradeIsAdditiveAndRecoverable`,
  `TestFleetReservationsThroughComposedCompanyHTTP` (46 HTTP assertions).
- Matrix and auditor walk (with a throwaway MinIO from
  `infra/compose.validation.yml` for the S3-backed suites) →
  `fleet007-matrix.jsonl`: 3/3 top-level, 3,378 subtests passed; matrix 220
  cells (85/135), auditor 118 routes (27 positive, 91 FORBIDDEN, 61 writes).
- `cmd/api` and the reservation/assignment apicontract tests pass.
- Added after GREEN as characterization (not counted as RED): HTTP
  registration pins the eight rules and refusal mapping
  (`TestReservationRegistrationPinsEightCompanyPolicies`,
  `TestReservationRefusalsMapToRegisteredCodes`), the job envelope
  (`TestReservationOutcomeJobEnvelope`) and the conflict envelope
  (`TestReservationConflictRevealsOnlyTheWindow`).
- Found by the union and fixed before commit: the left-service consumer's
  `revision=revision+1` needs `SELECT(revision)`; the first orgtest reduction
  grant lacked it and every existing FLEET-003 lifecycle test failed with
  "permission denied" (8 tests). The grant (and the README text) now include
  it, and the upgrade test proves the consumer under exactly the documented
  reduction columns (Fleet reservation grants revoked first).
- Union and untagged results: see "Validation".

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC1 precedence and every cause | domain `TestAvailabilityFollowsThePrecedenceTableAndListsEveryCause` (all six rows, cause order and content); PostGIS `TestFleetAvailabilityReadAndStartCheckPort` (AVAILABLE with both missing sources, RESERVED, BLOCKED + RESERVED, released after cancel, UNAVAILABLE after deactivation); HTTP availability body |
| AC2 overlapping confirmations | sequential: `TestFleetReservationCommandsConflictsAndReplay` (request and confirmation over a confirmed window → only that window; no row, receipt or audit left); concurrent, both orderings with barriers: `TestFleetReservationConcurrentConfirmationsAndCancel` (both commands observed waiting behind a held root lock; the first queued commits, the second gets `RESERVATION_CONFLICT` with exactly the winner's window; one CONFIRMED row); SQL 23P01 on the exclusion; HTTP details exactly `conflict_window` STARTS_AT/ENDS_AT with instants, no identifier/employee |
| AC3 job stopped → EXPIRED | `TestFleetReservationTimeOutcomesAtReadTimeAndByJob` (stored REQUESTED/CONFIRMED unchanged, reads EXPIRED at the start and NO_SHOW at the deadline; availability no longer RESERVED); domain `TestReservationOutcomesFollowTheClockAtReadTime` |
| AC4 start by someone else held | domain `TestStartDecisionHoldsWithAPreciseCode`; PostGIS start port (assigned member held `ASSET_UNAVAILABLE` inside another's confirmed window; the reserved employee admitted without assignment) |
| AC5 block during a trip | domain (IN_USE with BLOCKED as a further cause; next start held); PostGIS (block → next start held `ASSET_UNAVAILABLE`); the block command never touches reservations (`TestFleetReservationCommandsConflictsAndReplay`) |
| Reservation transition table (every row) | domain `TestReservationTransitionTableIsExactlyFleetSection64` (30 cells; FULFIL has no row); SQL `TestFleetReservationSchemaGuardsAndIsolation` (FULFILLED, early EXPIRED/NO_SHOW, reopen, revert and command-less transitions refused); store/HTTP for request, confirm, cancel, expire, miss, left service |
| Another organization's asset | store (`NOT_FOUND` for request and block, foreign employee `NOT_FOUND`, no row); HTTP 404 on the foreign asset and through the other organization's path; RLS test (other scope updates 0 rows, cross-org insert 42501) |
| Confirmation outside the member's scope | HTTP: Sul `UNIT` supervisor confirming on a Norte asset → 403 `FORBIDDEN` naming `fleet.reservation.manage`; Norte supervisor allowed; matrix `unit-scope` variant on the second-unit asset |
| Concurrent confirm and cancel | `TestFleetReservationConcurrentConfirmationsAndCancel` both orderings (confirm then cancel → CANCELLED; cancel then confirm → `CONFLICT`); final state CANCELLED either way |
| Block removed without a reason | application (`NewBlockRemove` refuses empty/blank); HTTP 422 missing and empty; SQL 23514; contract schema requires `reason` |
| Job run twice | handler run twice → one `NO_SHOW`, one `fleet.reservation.missed` event; expire job twice → one `EXPIRED`; settlement before the instant → `ErrNotDue`; malformed/mismatched jobs refused |
| Reservation over an asset that left service | open REQUESTED/CONFIRMED cancelled with `ASSET_LEFT_SERVICE` and the new asset revision on deactivation and on disposal (same transaction); already-expired one untouched; requests on INACTIVE/DRAFT/DISPOSED/ARCHIVED → `VALIDATION_FAILED`; blocks allowed on INACTIVE only besides ACTIVE |
| Replay / reuse | store and HTTP replay return the first result (`replayed: true`); same id + other intent → `OPERATION_ID_REUSED` |
| Forced RLS, append-only | SQL test (no rows without the setting, unscoped update 0 rows, DELETE and receipt UPDATE 55000) and the upgraded schema |
| Populated upgrade 215 → 217, failure and recovery | `TestFleetReservationPopulatedUpgradeIsAdditiveAndRecoverable` (ACTIVE/INACTIVE/DISPOSED assets, an assignment; injected failure leaves schema, trigger, usage source and data unchanged; ordinary migrator with only REFERENCES/TRIGGER/function ownership; no-op replay; previous image check; RLS, triggers, exclusion and allow-list lines; behaviour and left-service consumer on the upgraded schema) |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; a
throwaway MinIO project `f4-fleet007-s3` (torn down at the end).

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on fleet, orgaccess, organization,
  enterprisesuite, apicontract, orgapi, httpapi, cmd/api, cmd/worker,
  db/queries/fleet, and with `-tags=integration` on organization/adapters,
  fleet, orgtest, orgaccess: no findings (`fleet007-staticcheck.log`).
- `sqlc generate` (fleet stanza + 000217), `sqlc vet`, `sqlc diff` clean.
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  969 warnings / 633 informs vs 953 / 615 at `HEAD` (the existing route
  patterns for 8 new routes); patience diff is purely additive (+1,072 lines,
  one enum line changed).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff --check`
  clean; staged diff reviewed for secrets/PII: synthetic identifiers, the fixed
  synthetic cursor secret of existing tests and the disposable MinIO
  credentials only in the log environment file (not committed).

- Affected race/integration union `go test -json -race -tags=integration -timeout=60m -p 2
  ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/modules/fleet/...
  ./internal/modules/orgsync/... ./internal/modules/membership/... ./internal/modules/document/...
  ./internal/modules/orgexport/... ./internal/modules/fueling/... ./internal/platform/migrate/...
  ./internal/platform/enterprisesuite/... ./internal/platform/orgapi/... ./internal/platform/jobs/... ./cmd/api/`
  (with the throwaway MinIO) → `fleet007-db-union.jsonl`: 39 packages ok,
  1,091 top-level / 7,401 subtests passed, 0 failures, 0 skips
  (`organization/adapters` 1,161.8 s, two other agents' suites running on the
  same machine). The first attempt was stopped after the missing
  `SELECT(revision)` reduction grant was found (see GREEN) and rerun in full.
- Untagged whole module `go test -count=1 -timeout=45m -p 2 ./...` →
  `fleet007-untagged-all.log`: 214 packages ok, 78 without tests, 0 FAIL
  (includes apicontract, cmd/api, cmd/worker and the new unit tests).
- `cmd/worker` integration tests were not run: their 15 known failures need
  pinned Rust preparer CLIs this machine lacks (untouched packages); the
  worker's untagged tests (composition, dispatchers) pass.

## Retained

- Mission linkage, `FULFILLED` by a qualifying trip start and the early
  tolerance (EMP-TRIP-023); requesting through `trip.mission.manage` alone.
- The trip-start intake that calls `StartAvailabilityPort` and replaces
  `NoActiveUse` (EMP-TRIP-002), including the lifecycle-at-declared-instant
  rule for held starts; maintenance holds replace `NoMaintenanceHolds`.
- Notifications (requester on confirm/cancel/expire/no-show; managers on
  no-show) — no notification kind is registered here.
- A configurable no-show tolerance (15–240 minutes) as an organization setting.
- The calendar screen SCR-FLEET-06 (EMP-TRIP-022), panel and Android
  (FLEET-013 picker), reference feed (OFFL-006) availability fields.
- Production grants (README subsection), live worker composition review.
- No milestone, phase or gate is claimed.
