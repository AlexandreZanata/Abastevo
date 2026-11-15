# EMP-FLEET-003 — Asset lifecycle: acquisition, deactivation, disposal, revert, archive

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Sole bounded F2 task in worktree `codex/enterprise-f3-fleet003`, opened from
`b1489f2f` (F3 ledger) on top of PR203 `43b21389`. It is the first link of the
EDR-109 chain FLEET-003 → FLEET-005 → AUTH-006 → FLEET-006 → METER-002 closure
that gates every F3 persistence task. FLEET-002 (assets), METER-002 source
foundation (meters, readings) and OFFL-008 (actual held review) are LOCAL_DONE
and are the real dependencies used here; nothing is faked for them.

Read: task block `ep05-fleet-meters.md` EMP-FLEET-003; `modules/fleet.md` §3,
§4 (BR-FLEET-008..011, 027), §6.1, §6.2, §8, §9, §10.5; `08-state-model.md` §4;
EDR-109/110; evidence of FLEET-002, METER-002 and OFFL-008. Shared canonical
docs are not edited here: the rule/use-case bullets, EDR rows and backlog note
are frozen between the markers below for the coordinator to copy.

IDs used: B-BR-EMP-202..206, BUC-EMP-104..105, EDR-143..145 (all of them; no
pre-allocated ID is left unused). Migration `000201_*` (append-only; 000001..
000200 untouched).

## Rules frozen before behaviour

Current intake treatment observed before any change (source `meter_intake.go`,
`meters.go`, migrations 198/199): the signed `meter.reading.record` intake
never reads the asset lifecycle. A reading on a `DRAFT`, `INACTIVE`,
`DISPOSED` or `ARCHIVED` asset is classified only by retained permission
history and is applied (`ACCEPTED`) whenever the meter segment contains the
declared instant. No `ASSET_UNAVAILABLE` hold exists; the only held code is
`PERMISSION_CHANGED`. The 197/198 asset guard admits only `DRAFT → ACTIVE`,
so no persisted asset can be `INACTIVE`, `DISPOSED` or `ARCHIVED` before 201.

Rules B-BR-EMP-202, B-BR-EMP-203, B-BR-EMP-204, B-BR-EMP-205, B-BR-EMP-206, BUC-EMP-104, BUC-EMP-105 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-FLEET-003 — Asset lifecycle: acquisition, deactivation, disposal and archive" (frozen before behaviour; text moved verbatim at integration).

### Decisions (EDR rows, frozen before behaviour)

Decisions EDR-143, EDR-144, EDR-145 are recorded in [16-decisions](../16-decisions.md).

## Implementation

- Pure domain `fleet/domain/lifecycle.go`: the §6.1 table (`Transition`), closed
  `DisposalKind`, `RequiresReason`, `ChecksOpenWork`, the shared 280-scalar
  `ValidReason`. Go only (no Kotlin mirror was requested).
- Application `fleet/application/lifecycle.go`: `AssetLifecycle` command (exact
  shape per command, UTC normalisation, scoped SHA-256 intent digest with the
  exact nanosecond instant), `AssetOpenWork` port with `OpenTrip`/`OpenWorkOrder`,
  `OpenOperationsError`, `AssetLifecycleService.Transition/History` behind the
  existing current-decision checks, `LifecycleEntry` validation; `AssetView`
  gains `AcquiredAt`/`Disposal` with the DISPOSED/ARCHIVED ⇔ disposal invariant.
- Migration `000201_organization_asset_lifecycle.sql`: additive columns and
  checks on `org_assets`, extended `org_asset_commands` kind/result checks,
  forced-RLS immutable `org_asset_lifecycle_events` with source trigger and
  revision/receipt/audit FKs, a deferred "transition has history" constraint
  trigger, and `CREATE OR REPLACE` of `org_asset_guard` (§6.1 rows, ARCHIVED
  terminal, profile/disposal/acquisition immutability, reactivation evidence and
  unit admission, out-of-service assets may keep an archived unit),
  `org_meter_observation_guard` (live branch refuses at/after disposal) and
  `org_meter_held_source` (ASSET_UNAVAILABLE needs an actual disposal).
- Store `fleet/adapters/lifecycle.go`: `AssetStore.TransitionAsset` (authority →
  root lock → receipt replay → one audit event → CAS/state → instants vs DB
  clock → reactivation checklist → open-work port → update → snapshot → history
  → receipt → authority), `ListAssetLifecycle`, explicit `NoOpenWork`,
  `MeterStore.MeterAssetDisposalTx` (root then asset row lock). `loadAsset`,
  revision snapshots (fields omitted when absent, so pre-201 snapshot bytes are
  unchanged) and `AssetAuditDefinitions` carry the lifecycle data/actions.
- Intake `meter_intake.go`/`meter_review_hold.go`: after APPLY classification the
  disposal port decides `ASSET_UNAVAILABLE`; held code allow-list is exactly
  `PERMISSION_CHANGED`/`ASSET_UNAVAILABLE`; the review view accepts both.
- HTTP `fleet/adapters/http/lifecycle.go` + `assets.go`: six command routes and
  the history list through `guarded.Register` (fresh `fleet.asset.manage` /
  `fleet.asset.read`), per-command strict key sets, `orgapi.AssetHasOpenOperations`
  (409, `open_operations` details), view `acquired_at`/`disposal`.
- Composition `enterprisesuite/routes.go`: `OpenWork: fleetpg.NoOpenWork{}` and
  `RegisterAssetLifecycle`. OpenAPI: 7 paths, 7 schemas, view properties, error
  code, review code. Inventory: 6 manage + 1 read routes (normal routes 82→89).

Observed current behaviour retained: readings on `DRAFT`/`INACTIVE` assets are
still applied normally; reverted disposals stop holding; allocation periods stay
open across INACTIVE/DISPOSED/ARCHIVED (unit usage counts only ACTIVE assets).

## Validation

Environment: worktree backend, Go 1.27.4 toolchain
(`/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin`),
`GOCACHE=/data/dev/cache/f3-go-build` (later also `GOTMPDIR`/`TMPDIR=/data/dev/cache/f3-tmp`),
owned disposable PostGIS control `127.0.0.1:32775` (project emp-f2-20261113) used
only through orgtest isolated child databases and ordinary runtime roles.

### RED (failing first, against compiled stubs / unchanged code)

- Unit RED `go test -json -race -count=1 ./internal/modules/fleet/... ./internal/platform/orgapi/`
  → `/data/dev/worktrees/f3-logs/fleet003-unit-red.jsonl`: 16 new top-level and
  13 subtests failed (domain table/kind/reason, application command/digest/
  service/history/view, HTTP registration/DTO/refusal, intake disposal hold and
  precedence, orgapi open-work code); 111 existing top-level / 711 subtests passed.
- PostGIS schema RED (no migration):
  `go test -json -race -tags=integration -timeout=15m ./internal/modules/organization/adapters -run '^TestFleet(Lifecycle|Disposal)' -count=1`
  → `/data/dev/worktrees/f3-logs/fleet003-db-red-schema.jsonl`: 11/11 top-level
  failed (42P01 `org_asset_lifecycle_events`; missing `000201` file), 32.7s.
- PostGIS behaviour RED (migration present; store/HTTP/intake still stubs), same
  command → `/data/dev/worktrees/f3-logs/fleet003-db-red-behaviour.jsonl`: 10
  top-level and all 30 matrix subtests failed (`source unavailable`, routes
  absent, readings at the disposal instant applied); the fabricated-hold negative
  passed because the unchanged Go hold refused every non-PERMISSION_CHANGED code.
  An earlier attempt of this run hung because the concurrency test kept its
  barrier transaction open after a stub failure; the test now always releases it.
- Contract RED (console, not retained as a file): with routes registered and the
  YAML unchanged, `go test -count=1 ./cmd/api/ ./internal/platform/apicontract/`
  failed `TestFullProductionRouterEnterpriseContractAndDefaultOff`,
  `TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked` and every
  apicontract route gate with "unguarded or undocumented organization route".
- Historical rehearsal RED found by the first union: the 198 and 199 upgrade
  rehearsals ran the current store (whose `GetAsset` now reads 201 columns) on
  198-only schemas. Their subject assertions are unchanged; the 198 rehearsal now
  applies the remaining embedded chain before its current-code smoke, and the
  199 rehearsal writes its schema198 meter with exactly the InstallMeter effects
  through the 198 guards (`pre201InstallMeter`).

### GREEN

- Unit race + coverage:
  `go test -json -race -count=1 -coverprofile=... ./internal/modules/fleet/... ./internal/platform/orgapi/ ./internal/platform/apicontract/`
  → `/data/dev/worktrees/f3-logs/fleet003-unit-green.jsonl`: 191 top-level /
  1362 subtests passed, 0 failures, 0 skips (apicontract 509s under race).
- Focused PostGIS race (final head):
  `go test -json -race -tags=integration -timeout=14m ./internal/modules/organization/adapters -run '^Test(FleetLifecycle|FleetDisposal|MeterPopulatedOrdinaryUpgrade|MeterReviewPopulatedLegacyUpgrade)' -count=1`
  → `/data/dev/worktrees/f3-logs/fleet003-db-focused-2.jsonl`: 14 top-level / 30
  subtests passed, 0 failures/skips; composed HTTP test 42 request assertions.
- Coverage: unit `fleet/domain/lifecycle.go` 100%, `fleet/application/lifecycle.go`
  93.0%; PostGIS run with `-coverpkg=./internal/modules/fleet/...`
  (`/data/dev/worktrees/f3-logs/fleet003-db-coverage.out`) gives store
  `adapters/lifecycle.go` 82.2%, HTTP `adapters/http/lifecycle.go` 76.4%; merged
  unit ∪ PostGIS over the four lifecycle files 85.6%. Package unit totals:
  fleet/domain 98.1%, fleet/application 84.3%.
- Affected PostGIS race union at the final head:
  `go test -json -race -tags=integration -timeout=25m ./internal/modules/fleet/... ./internal/modules/organization/adapters ./internal/modules/orgsync/... -count=1`
  → `/data/dev/worktrees/f3-logs/fleet003-db-union-final.jsonl`: 10 packages ok,
  510 top-level / 3573 subtests passed, 0 failures, 0 skips (organization
  adapters 592.9s), including every pre-existing fleet, meter, review, intake,
  unit/team/employee, authorization-matrix and upgrade rehearsal test. The first
  union (`fleet003-db-union-1.jsonl`) had exactly the two historical rehearsal
  failures described above and nothing else.
- Static/format/generation: `gofmt -l ./internal ./cmd ./db` empty;
  `GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck` on fleet, orgapi,
  enterprisesuite, apicontract, cmd/api (untagged) and on organization/adapters +
  fleet with `-tags=integration`: no findings; `sqlc generate` (v1.31.1, fleet
  stanza now lists 000201) then `sqlc vet` and `sqlc diff` clean; `vacuum lint -r
  contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check`
  passed with 0 errors (915 warnings / 583 informs vs 901/570 at HEAD; the new
  ones are the same classes as the existing fleet paths: global tag, POST
  request-body and duplicate-description notices); `python3
  docs/enterprise/tools/check_plan.py` 348 tasks / 545 rules / 167 use cases /
  145 screens, 0 problems; `git diff --check` clean.
- Whole backend (what CI runs): `go build ./... && go vet ./...` clean, then
  `go test -json -count=1 ./...` → `/data/dev/worktrees/f3-logs/fleet003-untagged-all.jsonl`:
  190 packages ok (72 without tests), 2411 top-level / 5388 subtests passed,
  0 failures; 4 pre-existing opt-in skips (identity `TestGenerateVectors`,
  portalgen `TestRealOfficialSnapshotPages`/`TestWriteSEOFixtureSnapshot`,
  quality `TestPMQCMethodologyAgainstRealOpenData`).

### Acceptance and negatives → tests

| Case | Test (organization/adapters unless noted) |
|---|---|
| AC1 every §6.1 row succeeds from its source, CONFLICT from all others (6 commands × 5 states, exactly one audit/receipt/revision/history, snapshot+action agree, stale revision) | `TestFleetLifecycleEveryTransitionAndRefusalOnRealPostGIS` (30 subtests); pure `fleet/domain TestLifecycleTableTransitionsAndRefusalsForEveryRow` |
| AC2 open work → `ASSET_HAS_OPEN_OPERATIONS` naming kinds; nil/failed port fails closed; port not asked outside BR-FLEET-010 | `TestFleetLifecycleOpenWorkRefusesRemovalFromServiceNamingKinds` (test-only adapter); HTTP mapping `fleet/adapters/http TestLifecycleRefusalsAndProjectionsStayBoundedAndExact`; `orgapi TestAssetOpenOperationsNamesOnlyRegisteredDistinctKinds` |
| AC3 archive frees plate/fleet number; disposed does not | `TestFleetLifecycleArchiveFreesIdentifiersAndKeepsTheArchivedRecord`; HTTP test |
| AC4 disposed asset's asset/meters/readings/history readable | `TestFleetLifecycleRevertKeepsDisposalHistoryAndRecordsStayReadable` |
| AC5 revert → INACTIVE, disposal kept in history, dispose again | same test; HTTP test history pages |
| Concurrent deactivate + dispose: one winner | `TestFleetLifecycleConcurrentDeactivateAndDisposeLeaveOneWinner` (admin root-lock barrier, both runtime transactions observed in `pg_stat_activity` lock wait, no sleeps) |
| Missing/blank reason; unknown kind; future/before-acquisition instants | application `TestLifecycleCommandShapeFollowsSpecificationRows`; store instants in AC4/5 test; HTTP 422s; SQL 23514 in schema test |
| Archive straight from ACTIVE (and DRAFT) | AC1 matrix; HTTP test |
| Another organization's asset | HTTP `otherBase/.../dispose` → 404, no effect |
| Role without `fleet.asset.manage` | HTTP FINANCE → 403 with `permission=fleet.asset.manage`, history read 200; capability off → 403 write, 200 read |
| Replayed command | `TestFleetLifecycleReplayReturnsTheFirstResultAfterLaterChanges`; HTTP replay/`OPERATION_ID_REUSED` |
| Field edits / fabricated history / ARCHIVED terminal / immutable history (SQL) | `TestFleetLifecycleSchemaRefusesFieldEditsAndFabricatedHistory`; AC4/5 test (55000) |
| Populated 200→201 upgrade: failure injection, recovery, no-op, previous image, no invented data, validated constraints, forced RLS, old receipts replay, refusals | `TestFleetLifecyclePopulatedOrdinaryUpgradeFailureRecoveryAndPreviousImage` |
| BR-FLEET-011 kept: acquired DRAFT discardable | `TestFleetLifecycleAcquiredDraftRemainsDiscardable` |
| Post-disposal intake: D−1ns applied; D, D+1ns, D+1ms held `ASSET_UNAVAILABLE` (version-1 proof, no reading/effect, byte-identical replay); existing reviewer reads/accepts; live insert at D refused 23514; after revert applied again, earlier holds untouched | `TestFleetDisposalHoldsSignedReadingsAtOrAfterTheExactInstant`; unit `fleet/adapters TestMeterIntakeHoldsReadingsAtOrAfterTheExactDisposalInstant` |
| `PERMISSION_CHANGED` precedence; disposal source failure is UNAVAILABLE; undeclared held code refused; fabricated `ASSET_UNAVAILABLE` hold refused by SQL | `fleet/adapters TestMeterIntakeKeepsNormalHandlingWithoutDisposalAndPermissionPrecedence`; `TestFleetDisposalHoldCannotBeFabricatedForAnAvailableAsset` |

## Proposed shared-doc changes

- `backend/README.md`, after "Immutable meter source and exact time": add
  "### Asset lifecycle (EMP-FLEET-003)" stating that migration 201 adds nullable
  lifecycle columns to `org_assets` and the forced-RLS immutable
  `org_asset_lifecycle_events`; the Fleet principal needs `SELECT, INSERT ON
  org_asset_lifecycle_events` (table-level `org_assets` grants already cover the
  new columns); principals that insert meter readings or held readings need
  `SELECT (org_id, id, disposed_at, disposed_submicro) ON org_assets` if they only
  hold column grants; lifecycle routes reuse the enterprise flag, organizations
  capability and fresh company proof; `NoOpenWork` is wired until trips and work
  orders exist; nothing provisions live grants.
- `docs/enterprise/modules/fleet.md` §2 "Rest of F2": optionally note that the
  FLEET-003 open-work check is answered by an explicit none adapter until the trip
  and maintenance epics replace it (already a stated risk in the task block).
- `docs/enterprise/09-architecture.md` fleet module row, if it lists delivered
  routes: add lifecycle commands/history and migration 201.
- No change is needed to `10-offline-sync.md` (ASSET_UNAVAILABLE is already a
  defined review code) or to the Android app (it already renders the code).

## Retained obligations (not accepted here)

- Open-work sources: the trip epic (`TRIP` IN_PROGRESS/PAUSED) and the
  maintenance epic (`WORK_ORDER`) must replace `NoOpenWork` with real adapters
  and their own tests; until then BR-FLEET-010 is only as complete as "none".
- FLEET-005: closing/continuing unit (and cost-centre) allocation periods on
  deactivation/disposal; FLEET-006: ending assignments on the disposal (first
  real consumer; orgevent consumer registration belongs there); FLEET-007:
  cancelling reservations on deactivation/disposal.
- Derived availability (BR-FLEET-025/027, F4): holding field operations on
  `DRAFT`/`INACTIVE` assets, blocks, maintenance holds, reservations.
- Plan limit of active assets on reactivation (activation does not check it yet
  either); the "same physical asset returning is a new record linked to this
  one" link after archive; the panel screen SCR-FLEET-05.
- Reactivation with an archived unit answers NOT_FOUND (same mapping as the
  existing activation); a typed checklist would need a shared refinement.
- Native Android rerun for an `ASSET_UNAVAILABLE` receipt was not performed (the
  app already maps the code; no device was used in this task).
- Production grants/provisioning, remote CI, protected integration, M3/F3 and
  release acceptance remain separate; synthetic data only.
