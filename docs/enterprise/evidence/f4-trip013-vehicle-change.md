# EMP-TRIP-013 — Vehicle change within a mission

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task on `codex/enterprise-f4-trip009` (worktree
`/data/dev/worktrees/f4-trip009`), the second commit after EMP-TRIP-009 on
the F4 integration head `95c0e389`. Real dependencies used: the EMP-TRIP-008
mission store, authority and signed response; the EMP-TRIP-009 planning
revisions; EMP-TRIP-002 trips and the signed start/finish intake;
EMP-FLEET-002/003/006/007 assets, lifecycle, assignments and derived
availability (`StartDecisionDBTX`); METER-001 meters and readings; AUTH-006
structural scope; the audit runner and the company HTTP entry. Nothing is
faked; synthetic data only. No Android, panel, job or deployment change.

The TRIP-013 work was developed in a scratch copy of the tree
(`/data/dev/cache/f4-tmp/trip013-scratch`, outside Git) while the
EMP-TRIP-009 union ran on the worktree, so that run never compiled a moving
tree; the files were copied into the worktree after the EMP-TRIP-009 commit
`b854e4ad` and every gate below ran on the worktree. Base: `bff5a788` (the
coordinator's F4 head with EMP-CHECK-003 and EMP-TRIP-005) plus
EMP-TRIP-009.

Read: the EMP-TRIP-013 block and the TRIP-002/008/009/012/021/023 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3, 6, 8,
10; [fleet](../modules/fleet.md) §6.2 (derived availability) and the
FLEET-007 start decision; [08-state-model](../08-state-model.md) §§3, 4;
[07-permissions](../07-permissions.md) §7.3; [04-business-rules](../04-business-rules.md)
BR-TEN-004/005, BR-TIME-002, BR-AUD-001; [17-traceability](../17-traceability.md)
REQ-07.1/07.6; EDR-219, 228..231, 246..249; the TRIP-002, TRIP-008, TRIP-009,
FLEET-007, METER-001 and CHECK-003 evidence.

IDs used: B-BR-EMP-387..389, BUC-EMP-195..196, EDR-250..251 — all of them;
no pre-allocated ID is left unused. Migration
`000232_organization_trip_mission_vehicles.sql` (needed: the mission head's
vehicle was immutable under the 000227/000231 guard); every test derives the
version from one file-name constant (`missionVehicleMigration`) and builds
the previous image as `name < 000232` (it must contain 000227 and 000231).

## Rules frozen before behaviour

Observed before any change: the mission head guard refused any `asset_id`
change; no mission–trip link exists (EDR-230), so "the mission's running
trip" can only be a trip the assignee runs on the mission's current vehicle;
TRIP-002 trips and METER readings are bound to their asset and meter and
are update-protected (`org_trip_completion_guard`, `org_asset_keep_history`);
FLEET-007's `StartDecisionDBTX` returns a member-independent assessment
(`fleet.Assess`) beside the starter-dependent admission; the latest asset
usage body is 000230 (EMP-CHECK-003); `DRAFT` exists only at revision 1
(B-BR-EMP-353, EDR-228, the head shape CHECK).

Rules B-BR-EMP-387..389 and use cases BUC-EMP-195..196 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-013 — Vehicle change within a mission (2026-11-15)"; decisions
EDR-250 (server-only domain operation, running trip, availability) and
EDR-251 (persistence, guards, route) in [16-decisions](../16-decisions.md).

Decisions worth reading first:

- **Not a lifecycle command (EDR-250).** `Mission.ChangeVehicle` keeps the
  state, assignee, answer and plan and moves the revision by one; the
  seven-command vocabulary, `MissionTransition` and `mission-v2.psv` are
  unchanged and no Kotlin mirror is added (the app only reads the current
  vehicle). Allowed from `PLANNED`, `ASSIGNED`, `ACCEPTED` and `DECLINED`;
  `DRAFT` and the terminal states `INVALID_TRANSITION`; `IN_PROGRESS`
  `UNSUPPORTED_COMMAND`; the current vehicle again `INVALID_TRANSITION`. An
  answer still in flight for the earlier revision is `CONFLICT`.
- **Running trip (EDR-250).** A change is refused `CONFLICT` while the
  mission's assignee runs a trip on the current vehicle: that trip finishes
  on its own vehicle with its final reading, and work on the new vehicle is
  a new trip with the new vehicle's own reading. Nothing moves between assets.
- **New vehicle (EDR-250).** `ACTIVE`, not an `IMPLEMENT` (`NOT_FOUND` for
  unknown or foreign, else `VALIDATION_FAILED`); `IN_USE` (another trip runs
  it) `CONFLICT`; `UNAVAILABLE`/`IN_MAINTENANCE`/`BLOCKED` at the planned
  start `VALIDATION_FAILED`; reservations and a missing primary assignment are
  left to the trip start decision. A current assignee must still hold
  `trip.trip.start` covering the mission with the new vehicle; the planner's
  `trip.mission.manage` must cover the mission before (route authority) and
  after (`CoversMissionTx`).
- **Persistence (EDR-251).** Insert-only, forced-RLS
  `org_trip_mission_vehicles` bound to the `VEHICLE` transition's receipt
  key; the head guard admits a vehicle change only with that record; the
  record guard re-proves the previous vehicle, the new vehicle's lifecycle,
  the one-active-trip rule and the assignee's running trip.

## Shape

- Pure Go `trip/domain/mission_vehicle.go`: `VehicleChangeAllowed`,
  `Mission.ChangeVehicle` (fixed failure order: scope → vehicle reference →
  reason → positive expected revision → `CONFLICT` → state → same vehicle).
- Application `trip/application/missions.go`: `MissionVehicleCommand`
  (`VEHICLE`: mission, expected revision, asset, reason; the EMP-TRIP-008
  digest form binds them), `ErrMissionAssetBusy`,
  `ErrMissionTripInProgress`, `MissionAuthority.CoversMissionTx`, ack
  validation.
- Migration `000232_organization_trip_mission_vehicles.sql` (EDR-251):
  widened kind/state CHECKs by name; `org_trip_mission_vehicles` with its
  source guard; `org_trip_mission_next` (231 body plus the `VEHICLE` row),
  `org_trip_mission_guard` and `org_trip_mission_complete` (231 bodies plus
  the `VEHICLE` clauses; the one guard line that refused any asset change now
  refuses it unless the record of the new revision exists), and
  `org_asset_current_usage` from the 000230 body with the two record keys
  (every 000230 line kept, checked by a line diff).
- sqlc `db/queries/mission` (stanza lists 000232):
  `MissionAssigneeTripRunning`, `InsertMissionVehicle`,
  `ChangeMissionVehicle` (only the vehicle and the revision move; head
  queries and `AdvanceMission` unchanged, so the 000227/000231 rehearsals
  are untouched).
- Store `trip/adapters/missions.go`: `vehicle` (references, planner
  coverage, lifecycle, domain, assignee coverage, running trip, FLEET-007
  assessment), the `VEHICLE` branch of `change` (transition → record → head →
  receipt under the root lock with one `trip.mission.vehicle_changed` audit
  event).
- Authority `organization/adapters/mission_authority.go`: `CoversMissionTx`
  (current `trip.mission.manage` candidates and `CoveringRules` over the
  mission with the new vehicle; a denial names the key).
- HTTP `trip/adapters/http/missions.go`: `POST
  /v1/orgs/{org_id}/missions/{mission_id}/vehicle/change` (strict body
  `schema_version`, `operation_id`, `revision`, `asset_id`, `reason`), the two
  new errors mapped to `CONFLICT`.
- Contract: one path and `EnterpriseMissionVehicleRequest` (patience diff
  purely additive, +106 lines); inventory `trip.mission.manage` seven routes;
  router test paths +1; route counts and auditor walk updated.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip013-*`. The pure domain was
test-first; the store, authority and HTTP adapters were drafted before their
PostGIS and router tests (the TRIP-002/008/009 deviation) and their RED was
obtained against stubbed entry points and the missing migration.

- Go domain (compiled stubs returning `errNotImplemented`) `go test -race
  -json -run TestMissionVehicle ./internal/modules/trip/domain/` →
  `trip013-go-domain-red.jsonl`: 1/1 top-level failing, 25/25 subtests.
- Router RED (route wired, the TRIP-009 spec) → `trip013-cmd-red.jsonl`: 3
  top-level failing (undocumented `…/vehicle/change`); contract RED →
  `trip013-contract-red.jsonl`: 1/2 failing
  (`TestTripMissionContractOwnerShapesAndGuards`; the plan shapes test is
  unaffected).
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration -p 2 ./internal/modules/organization/adapters -run
  '^TestTripMissionVehicle'` → `trip013-db-red-schema.jsonl`: 6/6 failing
  (42P01 or unavailable). A first attempt failed for the wrong reason (the
  test helper registered a second vehicle without a tank, so activation was
  refused before any mission code ran); the helper now registers the tank of
  the trip fixture and the RED was rerun.
- PostGIS behaviour RED (migration present; the `VEHICLE` command stubbed to
  zero values) → `trip013-db-red-behaviour.jsonl`: 5/6 failing at their first
  behaviour check (no refusal, no change, no lock wait); the schema test
  passed, as it exercises only the database guards, whose RED is the schema
  RED above.

## GREEN and REFACTOR

- Go domain → GREEN at the first implementation (25/25), later 26 subtests
  with the `DRAFT` rows below; `mission_vehicle.go` 100.0 % statements,
  package 99.8 % (`trip013-go-domain-cover.out`).
- PostGIS first GREEN (`trip013-db-green.jsonl`, all 21 `TestTripMission*`)
  → 16/21: every change of a `DRAFT` mission failed because the head shape
  CHECK keeps `DRAFT` at revision 1 (B-BR-EMP-353, EDR-228), which the first
  draft of B-BR-EMP-387 (written before behaviour) had overlooked by allowing
  `DRAFT`. The rule, EDR-250/251, the domain (`DRAFT` → `INVALID_TRANSITION`),
  the SQL `VEHICLE` row, the OpenAPI text and the tests were corrected to the
  frozen invariant (tests now plan a mission before changing its vehicle and
  assert the `DRAFT` refusal); the RED logs above predate the correction →
  `trip013-db-green-2.jsonl`: 21/21 top-level, 121 subtests.
- Characterization (not counted as RED):
  `TestMissionVehicleCommandIsExact` (application).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: the previous vehicle's history and readings remain distinct from the new vehicle's work | PostGIS `TestTripMissionVehicleChangeKeepsEachVehicleEvidence`: an accepted mission whose assignee runs a trip on vehicle A is refused (`ErrMissionTripInProgress`, nothing stored); after the finish with its final reading the change to B is accepted (state `ACCEPTED`, assignee, answer and plan kept, revision 5, one record `A>B`, one `VEHICLE:ACCEPTED>ACCEPTED` transition, one receipt, one `trip.mission.vehicle_changed` audit event); A's trips, finishes, meters and readings are byte-identical afterwards; the next trip starts on B with B's own reading; no trip reading belongs to another asset's meter |
| Reading reassigned to a different asset (database refusal) | the same test and the schema test: moving the finish/start reading to B's meter and moving the trip to B are refused 55000 (row presence asserted first) |
| Foreign or unavailable vehicle | `TestTripMissionVehicleChangeRefusals`: unknown and another organization's asset `ErrNotFound`; draft, implement and an administratively blocked vehicle (FLEET-007 `BLOCKED` at the planned start) ineligible; HTTP 404 / 422; SQL guard 23514 for implement and draft |
| Overlapping active trip on the new vehicle (one-active-trip rule) | the acceptance test: changing another mission to B while B runs a trip → `ErrMissionAssetBusy` (409 over HTTP); SQL guard `mission vehicle in use` |
| In-progress trip interaction | refused while the assignee runs a trip on the current vehicle (store and SQL `mission trip in progress`); `IN_PROGRESS` missions `UNSUPPORTED_COMMAND` (domain) and no SQL `VEHICLE` row for them |
| Coverage and isolation | the refusals test: an ASSET-scoped DRIVER assignee of A blocks a change to another vehicle (ineligible); an ASSET-scoped planner of A is `ErrPermissionDenied` changing to another vehicle, a UNIT planner is admitted, AUDITOR denied; another organization `ErrNotFound`; HTTP: driver and auditor 403, UNIT planner on another unit's mission 403, another organization 404 |
| State rules, CAS, replay | domain table (all states, order); store and HTTP: `DRAFT`, cancelled and same-vehicle `INVALID_TRANSITION`/409, stale 409, replay `replayed=true`, reuse `OPERATION_ID_REUSED`; an answer in flight for the earlier revision `CONFLICT`, the current revision accepted |
| Concurrency | `TestTripMissionVehicleChangeRacesAssignment`: a change and an assignment on one revision queued behind a held root, both orders; the first queued wins and the loser is `CONFLICT` with nothing stored |
| History, guards, RLS | `TestTripMissionVehicleSchemaIsForcedRLSGuardedAndAppendOnly`: forced RLS, invisible without the setting and to another organization; update/delete/truncate 55000; refusals by guard or constraint name (no transition, previous not current, another planner, implement, draft, busy, same vehicle, assignee running a trip, a `VEHICLE` transition without its record, on a `DRAFT`, moving the state, on a terminal mission); a head whose vehicle moves without its record 55000; the SQL `VEHICLE` rows; the asset usage keys present and answers unchanged |
| Populated upgrade with failure and recovery | `TestTripMissionVehiclePopulatedUpgradeFailureRecoveryAndPreviousImage` (previous image `< 000232` containing 000227 and 000231; completed and in-progress trips; DRAFT, PLANNED (with plan), ACCEPTED and CANCELLED missions; injected failure leaves schema, the four replaced functions, the two tables' constraints and data untouched; ordinary migrator; recovery once, no-op replay, previous image compatible; usage answers unchanged; every asset allow-list line kept; a pre-upgrade plan replays; changes refuse the draft and the cancelled mission and work on the planned and accepted ones) |

## Validation

Environment: worktree backend at `b854e4ad` (EMP-TRIP-009 on `bff5a788`)
plus this task, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; the
coordinator MinIO (`127.0.0.1:32819`) for the S3-backed suites.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean; `GOTOOLCHAIN=go1.27.1 staticcheck` on trip,
  organization, enterprisesuite, apicontract, cmd/api and
  `db/queries/mission`, and with `-tags=integration` on trip,
  organization/adapters and orgtest: no findings (`trip013-staticcheck.log`).
- OpenAPI `vacuum lint`: 0 errors, 1,064 warnings / 660 informs vs 1,061 /
  660 at `b854e4ad` (`trip013-vacuum*.log`); patience diff purely additive
  (+106 lines).
- Route counts: 147 normal routes (routed 28 / not routed 56), 25 special;
  AUTH-011 auditor walk 147 / 30 / 117; authorization matrix 280 cells (112
  allowed / 168 denied); 8 signed operation owners.
- Go trip packages under race pass; `cmd/api` 22/22 top-level
  (`trip013-cmd-green.jsonl`).
- Kotlin not touched by this task (the app reads only the mission's current
  vehicle); the merged-tree enterprise suite of EMP-TRIP-009 (3,541 tests, 0
  failures) stands.
- Affected race/integration union (the EMP-TRIP-009 command and package
  list, with MinIO) → `trip013-db-union.jsonl`: 24 packages (23 ok,
  `orgtest` has no tests), 1,092 top-level / 9,310 subtests passed, 0
  failures, 0 skips, 24.8 min (`organization/adapters` 1,430 s,
  `apicontract` 1,234 s).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `trip013-untagged-all.log`: exit 0, 223 packages ok, 81 without tests, 0
  FAIL.
- `cmd/worker` integration tests were not run (untouched; pinned Rust
  preparer CLIs absent); nothing here composes a worker job.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII (synthetic data only).

## Retained

- Changing the vehicle of an `IN_PROGRESS` mission once the trip link exists
  (EMP-TRIP-021/023), and the app showing the current vehicle (EMP-TRIP-021).
- Driver handover inside a running trip (EMP-TRIP-012) and planned stops and
  legs (EMP-TRIP-010).
- A read route for the vehicle history of a mission (the panel board,
  EMP-TRIP-022) and a vehicle chosen for an unplanned `DRAFT` (plan first).
- Reservations for a mission's vehicle (EMP-TRIP-023, EDR-195).
- Production grants (README subsection) and live composition review.
- No milestone, phase or gate is claimed.
