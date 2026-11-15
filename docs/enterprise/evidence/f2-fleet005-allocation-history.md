# EMP-FLEET-005 — Unit and cost-centre allocation history

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Sole bounded F2 task in worktree `codex/enterprise-f3-fleet005`, opened from
the integrated FLEET-003 commit `59b57c19`. It is the second link of the EDR-109
chain FLEET-003 → FLEET-005 → AUTH-006 → FLEET-006 → METER-002 closure that
gates every F3 persistence task. FLEET-002 (assets and their initial unit
allocation), ORG-005 (units), ORG-006 (cost centres) and FLEET-003 (lifecycle)
are the real dependencies used here; nothing is faked for them.

Read: task block `ep05-fleet-meters.md` EMP-FLEET-005 (and FLEET-008, plus
`ep03-identity-access.md` EMP-AUTH-006, which consume the port);
`modules/fleet.md` §3, §4 (BR-FLEET-015/016/017), §6.1, §7, §8;
`07-permissions.md` §4 (BR-AUTH-004/005) and §10 example 3;
`06-domain-model.md` §6; evidence of FLEET-002 and FLEET-003; migrations
000174/000175 (unit and cost-centre admission and usage guards), 000197
(assets, the single `unit_id` column and the immutable
`org_asset_unit_allocations` initial period), 000199 (current usage guards)
and 000201 (lifecycle). Shared canonical docs are not edited here: the
rule/use-case bullets, EDR rows and backlog note are frozen between the
markers below for the coordinator to copy.

IDs used: B-BR-EMP-223..226, BUC-EMP-114..115, EDR-154..155 (all of them; no
pre-allocated ID is left unused). Migration `000206_*` (append-only; applied
migrations untouched; 000203..000205 are reserved by parallel tasks and the
coordinator may renumber this file at integration — every test derives the
version from one file-name constant and builds the previous chain as
`name < thisMigration`).

## Rules frozen before behaviour

Current storage observed before any change (migrations 197/199/201, source
`store.go`, `codec.go`): an asset stores one optional `unit_id`. Activation
(`DRAFT → ACTIVE`) inserts exactly one immutable `org_asset_unit_allocations`
row whose `started_at` the database sets to `clock_timestamp()` and whose
`ended_at` stays NULL forever (statement trigger refuses UPDATE/DELETE/
TRUNCATE). FLEET-003 lifecycle commands never touch it, so every asset that
ever left `DRAFT` has exactly one open row equal to its `unit_id`, and a
`DRAFT` asset has none. No asset stores a cost centre; the cost-centre usage
guard still answers `assets: 0` and fails closed on any unknown foreign key.
The unit usage guard counts open rows of `ACTIVE` assets.

Rules B-BR-EMP-223, B-BR-EMP-224, B-BR-EMP-225, B-BR-EMP-226, BUC-EMP-114, BUC-EMP-115 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-FLEET-005 — Unit and cost-centre allocation history" (frozen before behaviour; text moved verbatim at integration).

### Decisions (EDR rows, frozen before behaviour)

Decisions EDR-154, EDR-155 are recorded in [16-decisions](../16-decisions.md).

### Backlog note


## Implementation

- Migration `000206_organization_asset_allocation_history.sql`: `CREATE
  EXTENSION IF NOT EXISTS btree_gist`; forced-RLS `org_asset_unit_periods` and
  `org_asset_cost_centre_periods` (`valid_from`, nullable `valid_to`,
  `recorded_at`, PK `(org_id, asset_id, valid_from)`, composite
  same-organization FKs to `org_assets`, `org_units`, `org_cost_centres`,
  finite/ordered microsecond bounds check, GiST exclusion
  `*_no_overlap`); the guarded data move (consistency check that raises 23514
  `asset allocation history source inconsistent`, then one open period per
  initial allocation with `valid_from = recorded_at = started_at`) while forced
  RLS is lifted only inside the atomic migration; row guards (insert: scope,
  `org_unit_admit`/`org_cost_centre_admit` under the root lock, asset found
  (23503) and not `DRAFT`, start not before the first unit period (23514),
  recorded instant stamped; update: only closing an open period once under the
  root lock, otherwise 55000); statement guards refusing DELETE/TRUNCATE (55000);
  the after-insert trigger on `org_asset_unit_allocations` that opens the first
  unit period; the deferred chain check on unit periods and on `org_assets`
  state changes (`WHEN OLD.state IS DISTINCT FROM NEW.state`); additive
  `CREATE OR REPLACE` of `org_asset_current_usage` (199 body + two FK
  allow-list entries), `org_unit_current_usage` (199 body; asset count from
  current-or-future unit periods) and `org_cost_centre_current_usage` (175
  body; first actual `assets` count).
- Pure domain `fleet/domain/allocation.go`: `AllocationPeriod` (`[From, To)`,
  exact microsecond bounds, UTC, non-aliasing), `AllocationInstant` (microsecond
  floor, year 1..9999), `Contains`, `Open`, `ValidAllocationPage` (order,
  overlap, open-only-last, unit contiguity).
- Application `fleet/application/allocation.go`: `ErrUnallocated`,
  `AllocationHistory` (`UNIT`, `COST_CENTRE`), `AllocationEntry`/`Filter`/
  `Page`, `AllocationHistoryService.History` (current `fleet.asset.read` before
  and after the repository, full page validation, clone),
  `AllocationSnapshot` + `NewAllocationSnapshot`, and the
  `AllocationSnapshotPort` interface (`UnitAndCostCentreAt`,
  `UnitAndCostCentreAtTx`).
- Store `fleet/adapters/allocation.go`: `AssetStore.ListAssetAllocations`
  (authority → visible asset → start keyset → authority) and the port: one
  lock-free `AllocationAt` statement (sqlc `db/queries/fleet/allocations.sql`),
  `ValidateOrgTx` for the borrowed transaction, containment re-checked in Go,
  `ErrNotFound`/`ErrUnallocated`/empty cost centre. `storeError` keeps
  `ErrUnallocated`.
- HTTP `fleet/adapters/http/allocations.go`: `POST
  .../fleet/assets/{asset_id}/unit-allocations/list` and
  `.../cost-centre-allocations/list` through `guarded.Register` (current
  `fleet.asset.read`, strict page-only body, cursor bound to route, asset and
  limit, `valid_to` omitted while open). Composition
  `enterprisesuite/routes.go` registers them on the same `AssetStore`.
- Contract: 2 paths and 5 schemas in `contracts/openapi/v1.yaml`; inventory
  `contracts/testdata/enterprise/authorization-routes-v1.json` +2
  `fleet.asset.read` routes (normal routes 89→91); router test paths +2.
- Test fixtures adapted (no assertion weakened): `fleetStore` grants the period
  tables only when installed; `orgtest.GrantFleetUsage` grants the exact usage
  columns only when installed; the synthetic unit and cost-centre usage
  overrides allow-list the new FKs (`to_regclass`, so earlier chains are
  unaffected).
- Unchanged on purpose: the Go activation path (still records the FLEET-002
  initial allocation), lifecycle commands, the asset view (`unit_id`,
  `initial_allocation`), the legacy column and table.

## Acceptance mapping

| Item | Evidence |
|---|---|
| AC1 the port returns the unit in effect at a past instant after later changes | `TestFleetAllocationPortReproducesPermissionsExampleThree` (07-permissions §10 example 3: Norte → Sul, stored March snapshot equals the port answer after the change) |
| AC2 an `ACTIVE` asset always has exactly one unit period | `TestFleetAllocationActivationOpensOneUnitPeriodThroughTheLifecycle` (activation, replay, 7 lifecycle commands); refusals "delete only period", "close only period", "gap after close", "second open unit period" in `TestFleetAllocationRefusesArchivedDraftAndBrokenPeriods` |
| AC3 the cost centre may be absent and is never invented | port answers empty cost centre (activation and example-3 tests, upgrade test); upgrade moves no cost centre; `TestAllocationSnapshotIsExactAndNeverInventsACostCentre` |
| AC4 the instant of change belongs to the new period | domain `TestAllocationPeriodIsHalfOpenAndExact`, `TestAllocationInstantFloorsToTheMicrosecondAndRefusesInvalidInstants`; port at activation instant, change instant, −1 µs and ±1 ns |
| Negative: unit or cost centre of another organization | `TestFleetAllocationHistoryThroughComposedCompanyHTTP` (23503 for foreign unit, foreign cost centre and foreign asset; 42501 for a foreign row; port across organizations not found; foreign asset/route 404) |
| Negative: archived unit (and archived cost centre) | `TestFleetAllocationRefusesArchivedDraftAndBrokenPeriods`; upgraded schema "upgraded archived unit" |
| Negative: overlapping periods under concurrency | `TestFleetAllocationOverlapUnderConcurrencyExclusionWins` (two real transactions, lock wait observed through `pg_blocking_pids`, no sleep; loser 23P01 on `*_no_overlap`, no effect) |
| Negative: removing the only unit period of an active asset | DELETE/TRUNCATE 55000, closing without successor 23514 (also on the upgraded schema) |
| Negative: RLS without organization setting | refusal test (0 rows visible, unscoped UPDATE affects 0 rows, INSERT 42501, port refuses an unscoped transaction); upgrade test |
| Populated upgrade, failure, recovery, refusal | `TestFleetAllocationPopulatedUpgradeMovesTheSingleUnitExactly` (DRAFT with/without unit, ACTIVE, INACTIVE in a since-archived unit, ARCHIVED; injected failure and an inconsistent source both leave schema, guards, forced RLS, extension state and data unchanged; recovery by an ordinary NOSUPERUSER/NOBYPASSRLS migrator; exact move; no-op replay; previous image check; previous-image activation opens the period; receipts replay; installed guards) |
| Existing unit-archive guard still blocks a unit with an open allocation | `TestFleetAllocationArchiveGuardsCountCurrentAndFuturePeriods` (P7703 current and future, P7803 cost centre, ended periods and out-of-service assets allowed) |
| Replayed command | No command is added (EDR-155); activation replay keeps one period |
| History routes in the contract | `TestFleetAllocationContractExamplesAndExactShapes`, `TestFleetAllocationContractBindsActualCurrentCompanyGuards`, router/inventory tests, `TestAllocationRegistrationPinsTwoCurrentReadPolicies` |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain
(`/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin`),
`GOCACHE=/data/dev/cache/f3-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f3-tmp`,
`STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`; owned disposable PostGIS
control `127.0.0.1:32775` used only through orgtest isolated child databases,
ordinary runtime roles and, for the upgrade, an ordinary NOSUPERUSER/NOBYPASSRLS
migrator role. Logs in `/data/dev/worktrees/f3-logs/fleet005-*`.

### RED (failing first, against compiled stubs / without the migration)

- Unit RED `go test -json -count=1 ./internal/modules/fleet/...` →
  `fleet005-unit-red.jsonl`: 9 new top-level tests failed (domain instant/
  period/page, application filter/history/snapshot, HTTP registration/strict
  body/projection); 102 existing top-level / 735 subtests passed.
- Contract RED `go test -json -count=1 ./internal/platform/apicontract/ -run
  TestFleetAllocation` → `fleet005-contract-red.jsonl`: 2/2 top-level failed
  (5 schema subtests missing; routes undocumented).
- Router RED `go test -json -count=1 ./cmd/api/ -run
  'TestFullProductionRouterEnterpriseContractAndDefaultOff|TestAuthorizationInventory'`
  → `fleet005-cmd-red.jsonl`: 2 failed (routes not mounted, inventory 89≠91),
  2 passed.
- PostGIS schema RED (no migration) `go test -json -race -tags=integration
  -timeout=15m ./internal/modules/organization/adapters -run '^TestFleetAllocation'
  -count=1` → `fleet005-db-red-schema.jsonl`: 7/7 failed (42P01 on the period
  tables; migration file missing), 28.5 s wall.
- PostGIS behaviour RED (migration present, Go stubs), same command →
  `fleet005-db-red-behaviour.jsonl`: 5 failed (port/history/HTTP stubs: source
  unavailable, route 404); the overlap-concurrency and archive-guard tests
  already passed because they exercise only the migration's SQL.

### GREEN

- Focused PostGIS race with coverage, same command plus
  `-coverpkg=./internal/modules/fleet/...` → `fleet005-db-focused-final.jsonl`:
  7/7 top-level passed, 0 failures/skips, package 11.3 s; composed HTTP test
  26 request assertions.
- Affected PostGIS race union `go test -json -race -tags=integration
  -timeout=45m ./internal/modules/fleet/... ./internal/modules/organization/adapters
  ./internal/modules/orgsync/... -count=1` → `fleet005-db-union-1.jsonl`:
  10 packages ok, 526 top-level / 3573 subtests passed, 0 failures, 0 skips
  (organization adapters 710.4 s), including every pre-existing fleet, meter,
  review, intake, lifecycle, unit/cost-centre/team/employee, authorization
  matrix and upgrade rehearsal test.
- Unit race + coverage `go test -json -race -count=1 -coverprofile=...
  ./internal/modules/fleet/... ./internal/platform/apicontract/ ./cmd/api/` →
  `fleet005-unit-green.jsonl`: 198 top-level / 1384 subtests passed, 0
  failures/skips (apicontract 522.1 s, cmd/api 21.6 s).
- Untagged whole module `go test -count=1 ./...` →
  `fleet005-untagged-all.log`: 196 packages ok, 72 without tests, 0 FAIL.
- Coverage (statements): `fleet/domain/allocation.go` 100%,
  `fleet/application/allocation.go` 98.1%, `fleet/adapters/http/allocations.go`
  51.5% unit / 88.2% unit ∪ PostGIS, `fleet/adapters/allocation.go` 78.9%
  (PostGIS; the uncovered statements are nil-store and corrupted-row
  fail-closed branches). Profiles `fleet005-unit-green-coverage.out`,
  `fleet005-db-coverage.out`.
- Build/vet/format/static/generation: `go build ./...`, `go vet ./...`,
  `go vet -tags=integration ./...` clean; `gofmt -l ./internal ./cmd ./db` empty;
  `GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck` on fleet, apicontract,
  enterprisesuite, orgapi, cmd/api and with `-tags=integration` on
  organization/adapters, fleet and orgtest: no findings; `sqlc generate`
  (v1.31.1, fleet stanza + 000206), `sqlc vet` and `sqlc diff` clean.
- OpenAPI: `vacuum lint -r contracts/openapi/vacuum-rules.yaml
  contracts/openapi/v1.yaml --no-update-check` → 0 errors (919 warnings / 590
  informs vs 915 / 583 at `HEAD`; the new ones are the existing route pattern:
  `enterprise-fleet` tag not global, POST list request body, duplicated
  common description).
- `git diff --check` clean; `python3 docs/enterprise/tools/check_plan.py` →
  0 problems. Secret/PII scan `git diff --cached | grep -nEi
  'password|secret|token|BEGIN .*KEY|@gmail|cpf'`: only the cursor-namespace
  field name `token`, the synthetic fixed test cursor secret
  (`bytes.Repeat([]byte{7}, 32)`, as in the FLEET-003 HTTP test) and
  runtime-generated synthetic session tokens of disposable test accounts; no
  credential, key, e-mail or personal datum. All data is synthetic.

## Retained obligations

- EMP-FLEET-008: the public transfer command with effective instant, reason,
  scope and destination checks, scheduling/listing/cancellation (needs an
  additive guard change: today a period can only be closed once and is never
  deleted), `fleet.asset.transferred`, the asset view's current allocation, the
  reactivation admission of the current period's unit and the retirement or
  re-pointing of the legacy `org_assets.unit_id` (still read by the meter held
  reading unit snapshot) and `org_asset_unit_allocations`.
- EMP-AUTH-006: unit/cost-centre scope evaluation using stored snapshots; no
  module stores a port snapshot yet (trips, fuelings, expenses, readings and
  documents adopt it when their persistence lands).
- Production grants (proposal 4 below), migration rollout with the
  `btree_gist` extension, panel screens SCR-FLEET-02/05 and release remain
  separate. No M3/F3 acceptance is claimed.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. Product contract: move the bullets between `CONTRACT:BEGIN`/`CONTRACT:END`
   verbatim under a new section after "Enterprise asset lifecycle
   (EMP-FLEET-003)", preceded by
   `## EMP-FLEET-005 — Unit and cost-centre allocation history (2026-11-14)`
   and the usual evidence link line pointing to this file.
2. `docs/enterprise/16-decisions.md`: insert the two rows between
   `EDR:BEGIN`/`EDR:END` in EDR number order.
3. `docs/enterprise/backlog/ep05-fleet-meters.md` EMP-FLEET-005: state
   `TODO` → `LOCAL_DONE` with the backlog note between `BACKLOG:BEGIN`/
   `BACKLOG:END`; EMP-FLEET-008 "Risks and assumptions" may add: "FLEET-005
   period guards allow only closing an open period once and never delete; a
   scheduled-transfer cancellation needs an additive guard change here."
4. `backend/README.md`, new subsection after "Asset lifecycle (EMP-FLEET-003)":

   ```markdown
   ### Allocation history (EMP-FLEET-005)

   Migration206 installs the trusted `btree_gist` extension (PostgreSQL
   contrib, PostgreSQL License, shipped in the pinned `postgis/postgis:18-3.6`
   image; the migrator needs CREATE on the database, which the database owner
   has) and the forced-RLS `org_asset_unit_periods` and
   `org_asset_cost_centre_periods` tables, and moves every activated asset's
   initial unit into its first period. The Fleet principal needs
   `SELECT, INSERT ON org_asset_unit_periods, org_asset_cost_centre_periods`
   (activation opens the first unit period through a trigger). Unit-source
   principals additionally need
   `SELECT (org_id, asset_id, unit_id, valid_to) ON org_asset_unit_periods`;
   cost-centre-source principals need `SELECT (org_id, id, state) ON
   org_assets` and `SELECT (org_id, asset_id, cost_centre_id, valid_to) ON
   org_asset_cost_centre_periods`. A module calling the snapshot port needs
   `SELECT (org_id, id) ON org_assets` and `SELECT` on both period tables. No
   UPDATE grant is needed until the EMP-FLEET-008 transfer (`UPDATE (valid_to)`);
   DELETE/TRUNCATE are refused by triggers regardless. History routes reuse the
   enterprise flag, the organizations capability and current
   `fleet.asset.read`. Nothing here provisions live grants.
   ```
5. `docs/enterprise/modules/fleet.md` §8 (optional clarification, after
   "Period invariants are exclusion constraints"): "Allocation boundaries are
   whole microseconds; a snapshot instant is floored to the microsecond
   (EDR-155)."
6. `scripts/check-manifest.txt`: no change (no new package).
