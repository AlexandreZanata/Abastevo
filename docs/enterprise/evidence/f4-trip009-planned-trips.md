# EMP-TRIP-009 — Manager-planned trips: destination, reason, customer or project, cargo or passengers

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task on `codex/enterprise-f4-trip009` (worktree
`/data/dev/worktrees/f4-trip009`), based on the F4 integration head
`95c0e389` (TRIP-001..004, TRIP-008 missions, TRIP-015/016, FLEET-007/011,
CHECK-001, METER-003/004, OFFL-006/009/012, catalog v5). Real dependencies
used: the EMP-TRIP-008 pure mission, store, authority (`MissionStructure`),
signed `trip.mission.respond` owner and routes; ORG-005/006 units and cost
centres; AUTH-006 structural scope; the audit runner and the company HTTP
entry. Nothing is faked; synthetic data only. No Android screen, panel, job
or deployment change.

Read: the EMP-TRIP-009 block and the TRIP-008/013/021/022 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§1–8;
[08-state-model](../08-state-model.md) §§1, 3, 4 (Mission row, rules 2, 3, 7);
[07-permissions](../07-permissions.md) §7.3 (`trip.mission.manage`);
[10-offline-sync](../10-offline-sync.md) §10.3 (missions planning is an
online panel action); [11-privacy-security](../11-privacy-security.md)
BR-PRIV-005/007/008; [04-business-rules](../04-business-rules.md) BR-AUD-001,
BR-TEN-005, BR-REV-001; [17-traceability](../17-traceability.md) REQ-07.1..07.3;
[06-domain-model](../06-domain-model.md) §3 (Mission → unit, cost centre);
[organizations](../modules/organizations.md) §§2–3 (cost centres per client
contract, project or route); EDR-161, 228..231, 236; the TRIP-008 evidence
(its Retained list names this task) and the TRIP-002/004/015 evidence;
`kernel.HasCPFShapedToken`.

IDs used: B-BR-EMP-381..386, BUC-EMP-192..194, EDR-246..249 — all of them;
no pre-allocated ID is left unused. Migration
`000231_organization_trip_mission_plans.sql` only; every test derives the
version from one file-name constant (`missionPlanMigration`) and builds the
previous image as `name < 000231` (it must contain 000227, 000229 and 000230).

## Rules frozen before behaviour

Observed before any change: `PLAN` was a reserved command answering
`UNSUPPORTED_COMMAND` in Go, Kotlin, SQL (`org_trip_mission_next`) and
`mission-v1.psv`; the head guard, the transition kind CHECK and the receipt
kind/state CHECKs admitted no `PLAN`; no customer, project or place registry
exists in the organization model, while cost centres model client contracts,
projects or routes and the domain model names the mission's cost centre; the
repository's privacy shape for free text is `kernel.HasCPFShapedToken`; the
latest usage bodies were 000227 (unit) and 000220 (cost centre) at `95c0e389`
and became the 000230 bodies (EMP-CHECK-003) once the coordinator's F4 heads
`8ce567b5` and then `bff5a788` were merged in before the final gates (see
Validation).

Rules B-BR-EMP-381..386 and use cases BUC-EMP-192..194 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-009 — Manager-planned trips: destination, reason, customer or
project, cargo or passengers (2026-11-15)"; decisions EDR-246 (table, plan
value, Kotlin mirror, oracle v2), EDR-247 (minimised content), EDR-248
(persistence, usage sources) and EDR-249 (route, projections, inventory) in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Decisions worth reading first:

- **Table (EDR-246).** `PLAN` from DRAFT, PLANNED, ASSIGNED, DECLINED →
  PLANNED, withdrawing the assignee and answer; from ACCEPTED
  `INVALID_TRANSITION` (an accepted plan is the member's commitment; the
  planner reassigns or cancels); IN_PROGRESS stays `UNSUPPORTED_COMMAND`
  (the trip link of TRIP-021/023); terminal `INVALID_TRANSITION`. An answer
  still in flight for a withdrawn assignment is `FORBIDDEN` (B-BR-EMP-356
  order: not the current assignee), a stale command `CONFLICT`.
- **Minimisation (EDR-247).** Destination = free text or an `ACTIVE` unit of
  the organization; reason required; customer or project = an optional
  `ACTIVE` cost centre; load = NONE, CARGO with a description, or PASSENGERS
  with only a count 1..999. Free texts refuse a standalone CPF-shaped token
  (Go `kernel.HasCPFShapedToken`, the same pattern in Kotlin and SQL). No
  passenger identity, description or contact, no coordinates, no customer
  name; unknown fields are refused by the strict body.
- **Plan beside the mission (EDR-246/248).** The plan is a separate value and
  an insert-only planning record keyed by the revision its `PLAN` produced,
  bound by a composite foreign key to that transition's receipt key; the head
  keeps unit, asset, title and window and never stores plan content; the
  current plan is the latest planning revision and survives assignment,
  response and cancellation. Head queries are unchanged (plans have their own
  queries), so the 000227 rehearsal still runs the store before 000231.
- **Oracle version (EDR-246).** `mission-v2.psv` supersedes `mission-v1.psv`
  (kept unchanged, no longer consumed): every v1 row byte for byte except the
  eight `PLAN` TRANSITION rows, plus LOAD, PLAN_VALUE and PLAN.

## Shape

- Pure Go `trip/domain/mission.go` (the `PLAN` rows of `MissionTransition`)
  and new `trip/domain/mission_plan.go`: `MissionLoad` (NONE, CARGO,
  PASSENGERS), `CostCentreRef`, `ValidPlanText`, `PlanInput`, `PlanSnapshot`,
  `Plan` (redacted, independent snapshots), `NewPlan` with the fixed failure
  order, `Mission.Plan(scope, expected, input)`.
- Kotlin mirror: `TripMissionLifecycle` `PLAN` rows and `TripMission.plan` in
  `TripMission.kt`; new `TripMissionPlan.kt` (`CostCentreRef`,
  `TripMissionLoad`, `TripMissionPlanText`, `TripMissionPlanInput`,
  `TripMissionPlan.create`).
- Oracle `contracts/testdata/enterprise/trip/mission-v2.psv` (317 cases:
  STATE 20, COMMAND 15, RESPONSE 8, TRANSITION 62, CREATE 29, RESTORE 34,
  ASSIGN 23, RESPOND 32, CANCEL 15, LOAD 8, PLAN_VALUE 50, PLAN 21; SHA-256
  `caf313daaee52b842e374ca091636f79a17b39d14f8728b6efa7f36d6572b95c`) and its
  README section. It was generated from an independent Python model of the
  frozen rules (`/data/dev/worktrees/f4-logs/trip009-oracle/gen2.py`, the
  TRIP-008 generator extended; not committed) before any implementation; the
  generator also reproduces `mission-v1.psv` byte for byte and cross-checks
  every accepted plan text against its own CPF/text model.
- Application `trip/application/missions.go`: `MissionPlanCommand` (`PLAN`),
  `MissionPlanFields` (scope-free validation mirroring the domain, `Domain`
  builder), a `trip.mission.plan.v1` digest for `PLAN` only (CREATE, ASSIGN
  and CANCEL keep the EMP-TRIP-008 digest, so stored receipts replay),
  `MissionPlanView` on `MissionView`, ack and view validation (a PLANNED
  mission must carry a plan).
- Migration `000231_organization_trip_mission_plans.sql` (EDR-248): widened
  CHECKs by name; `org_trip_mission_plan_text_valid`; forced-RLS insert-only
  `org_trip_mission_plans` with shape CHECKs and the BEFORE INSERT source
  guard; replaced `org_trip_mission_next`, transition source, head guard and
  deferred publication check (every 227 line kept except the one assignee
  clause widened from `kind<>'ASSIGN'` to `kind NOT IN('ASSIGN','PLAN')`);
  the 227 unit and 220 cost-centre usage bodies with one plan line each.
- sqlc `db/queries/mission` (stanza lists 000231): `MissionCostCentreState`,
  `InsertMissionPlan`, `MissionPlans` (latest plan per mission).
- Store `trip/adapters/missions.go`: `PLAN` in `change` (references → domain
  → transition → planning row → head → receipt, under the root lock with
  one `trip.mission.planned` audit event), plans attached to read, list and
  the own list.
- HTTP `trip/adapters/http/missions.go`: `POST …/missions/{mission_id}/plan`
  (closed nested `destination` and `load`, revision required) and the `plan`
  projection.
- Contract: one path and four schemas (`EnterpriseMissionDestination`,
  `EnterpriseMissionLoad`, `EnterpriseMissionPlanRequest`,
  `EnterpriseMissionPlan`), `plan` on the item and view (patience diff purely
  additive, +225 lines); inventory `trip.mission.manage` six routes; router
  test paths +1; `TestTripMissionPlanContractShapes`.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip009-*`. The pure domain and the
Kotlin mirror were test-first. The application, store and HTTP adapters were
drafted before their PostGIS and router tests (the TRIP-002/008 deviation);
their RED was obtained by freezing the tests and the migration and running
them against stubbed entry points. No expectation changed between RED and
GREEN except the one named below.

- Go domain (compiled stubs returning `errNotImplemented`, empty load
  vocabulary, the v1 table) `go test -race -json ./internal/modules/trip/domain/`
  → `trip009-go-domain-red.jsonl`: 20 top-level, 1 failing
  (`TestSharedTripMissionVectors`), 82/317 vectors failing (the five changed
  `PLAN` TRANSITION rows, 8 LOAD, 48 PLAN_VALUE, 21 PLAN); the two passing new
  rows (`plan-value-destination-unit-malformed`,
  `plan-value-project-malformed`) are refused by the scoped-reference
  constructor inside the fixture, as in v1.
- Kotlin (`TODO()` stubs) `:domain:test --tests '…trip.EnterpriseTripMission*'`
  (flock) → `trip009-kotlin-red.log`, XML in `trip009-kotlin-red-xml/`: 317
  vectors, 82 failures (the same two rows refused by `ScopedTripRef`). A first
  attempt still loaded `mission-v1.psv` and failed on the hash for the wrong
  reason (`trip009-kotlin-red-wrongfile.log`); the file name was fixed and the
  RED rerun.
- Router RED (route wired, the head spec) `go test -json ./cmd/api/` →
  `trip009-cmd-red.jsonl`: 3 top-level failing
  (`TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff`: undocumented
  `POST /v1/orgs/{org_id}/missions/{mission_id}/plan`). Contract RED →
  `trip009-contract-red.jsonl`: 2/2 failing.
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration -p 2 ./internal/modules/organization/adapters -run
  '^TestTripMissionPlan'` → `trip009-db-red-schema.jsonl`: 4/4 failing (42P01
  or unavailable); the upgrade test, written afterwards, failed the same way
  on the missing file → `trip009-db-red-upgrade.jsonl` 1/1.
- PostGIS behaviour RED (migration present; `CommandMission` for `PLAN` and
  the plan attachment stubbed to zero values) →
  `trip009-db-red-behaviour.jsonl`: 4/4 failing at their first behaviour
  check (no plan, no permission denial, no lock wait, stale revision).

## GREEN and REFACTOR

- Go domain → `trip009-go-domain-green.jsonl`: 21/21 top-level and 317/317
  vectors under race at the first implementation; characterization
  `TestPlanValuesAreRedactedAndSnapshotsIndependent` added; `mission_plan.go`
  100.0 % statements, package 99.8 % (`trip009-go-domain-cover.out`).
- Kotlin → `trip009-kotlin-green.log`: 317/317 vectors and 2/2 unit tests at
  the first implementation (`plansAreRedactedAndPlanningDoesNotMutate`
  added); whole enterprise domain suite 3,374/3,374 on `95c0e389`
  (`trip009-kotlin-enterprise-final.log`).
- Application: `TestMissionPlanCommandIsExactAndDigestStable`
  (characterization: exact kinds, every plan refusal, digest binding, the
  unchanged EMP-TRIP-008 digest form).
- PostGIS (`-race -tags=integration`, NOSUPERUSER / NOBYPASSRLS runtime role)
  → `trip009-db-green.jsonl`: 12/13 top-level at the first run (all TRIP-008
  mission tests and the plan schema, scope and race tests). The failing one
  exposed two test defects, fixed without touching the implementation: a
  second unit reused the `Synthetic South` name (P7701), and the test expected
  `CONFLICT` for an answer in flight after a re-plan although the frozen
  B-BR-EMP-356 order makes it `FORBIDDEN` (the member is no longer the
  assignee). That expectation — and the matching wording of BUC-EMP-193 and
  EDR-246, written before behaviour — were corrected to the frozen rule
  (`trip009-db-green-3.jsonl` 1/1). Upgrade test → `trip009-db-upgrade-1.jsonl`
  1/1; plan HTTP and TRIP-008 HTTP → `trip009-db-http-1.jsonl` 2/2.
- The synthetic usage fixture copies (`installSyntheticUnitUsageSource`,
  `installSyntheticCostCentreSource`, the FUEL-002 trap) list the two plan
  keys; `grantMissionTables` grants the plan table (and SELECT on cost
  centres) only once 000231 exists, so the 000227 rehearsal is unchanged.
- Router and contract: `cmd/api` 22/22 top-level
  (`trip009-cmd-green.jsonl`); apicontract mission tests pass.
- Merge consequence (not a behaviour change): 000230 (EMP-CHECK-003) also
  redefines the unit and cost-centre usage sources, so 000231 now starts
  from the 000230 bodies (every 000230 line kept, one plan line each, checked
  by a line diff); the upgrade test requires 000230 in the previous image and
  checks the incident keys survive.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: the planning revision records the authorized source | PostGIS `TestTripMissionPlanRecordsSourceAndStaysApartFromExecution`: one plan row, transition, receipt and audit event per plan; the row joins its `PLAN`/`COMPANY` transition, the `trip.mission.planned` audit event and the `PLANNED` receipt with the planner's account and membership; the database refuses a plan row by another planner, of another transition kind or without its transition (schema test) |
| AC: the plan stays distinct from actual execution | the same test snapshots trips, finishes, readings, logbook events, review entries, reservations, blocks, review items and assets around a completed trip before planning, after plan, replay, reuse, assignment, re-plan, acceptance and cancellation — unchanged; the head keeps unit, asset, title and window (domain `plan-keeps-content`); plans live only in `org_trip_mission_plans` |
| Foreign unit / cost centre | store: a unit or cost centre of another organization or unknown → `ErrNotFound`, archived → ineligible, nothing stored; HTTP 404 / 422; SQL guard 23514 for an unknown or archived cost centre |
| Structural scope FORBIDDEN / other organization NOT_FOUND | `TestTripMissionPlanScopeIsolationAndAuditor` (UNIT planner on the other unit's mission, AUDITOR and DRIVER `ErrPermissionDenied`; another organization `ErrNotFound`; heads unchanged); HTTP 403 for unit scope, driver and auditor, 404 through another organization's path; AUTH-011 walk: the plan route `FORBIDDEN` for AUDITOR with unchanged state |
| Unapproved sensitive data refused by validation | domain/Kotlin PLAN_VALUE and PLAN vectors (CPF-shaped tokens in each text, passenger description, cargo count, unknown load); application refusals; HTTP: a passenger identity or customer-name field `MALFORMED` (strict body), a CPF-shaped destination and a passenger description `VALIDATION_FAILED`; OpenAPI shape refuses passenger names, coordinates and customer names; SQL CHECKs refuse CPF-shaped texts, passenger descriptions, cargo counts and counts above 999 |
| A plan revision rewriting a completed trip or an accepted/in-progress mission | the completed trip is untouched (above); an accepted mission → `ErrInvalidTransition`, HTTP 409, nothing stored; a cancelled mission → `INVALID_TRANSITION`; IN_PROGRESS and COMPLETED → domain/Kotlin `UNSUPPORTED_COMMAND`/`INVALID_TRANSITION` vectors and the SQL table (`org_trip_mission_next` NULL for ACCEPTED, IN_PROGRESS, COMPLETED, CANCELLED); a PLAN transition on a terminal mission 23514 |
| Revision CAS, replay, reuse | stale and future revisions `CONFLICT` (store, HTTP 409); same operation and digest replays the first result; another digest `OPERATION_ID_REUSED` (store, HTTP) |
| Concurrency | `TestTripMissionPlanRacesAssignment`: a plan and an assignment on one revision queued behind a held root, both orders; the first queued wins, the loser `CONFLICT` with nothing stored |
| Re-plan withdraws a pending assignment | after a re-plan of an ASSIGNED mission the head has no assignee, the answer in flight is `FORBIDDEN` with nothing stored, the own list is empty, and the same member can be assigned again and accept |
| Audit and history | one `trip.mission.planned` audit event per plan; the transition history `CREATE,PLAN,PLAN,ASSIGN,PLAN,ASSIGN,ACCEPT,CANCEL` and planning revisions 2, 3 and 5 are kept; the cancelled mission still reads its last plan |
| Forced RLS, insert-only, guards | `TestTripMissionPlanSchemaIsForcedRLSGuardedAndAppendOnly` (forced RLS, invisible without the setting and to another organization; update/delete/truncate 55000; 14 insert refusals by guard or constraint name; a head reaching PLANNED without its plan 55000; usage sources recognise both keys, answers unchanged, planned unit and cost centre can still be archived and the history keeps them) |
| Populated upgrade with failure and recovery | `TestTripMissionPlanPopulatedUpgradeFailureRecoveryAndPreviousImage` (previous image `< 000231` containing 000227, 000229 and 000230; completed and in-progress trips; DRAFT, ASSIGNED, ACCEPTED and CANCELLED missions with receipts; injected failure leaves schema, the six replaced functions, the two tables' constraints and data untouched; ordinary migrator with table and function ownership and REFERENCES only; one recovery, no-op replay, previous image compatible; usage answers unchanged; every allow-list line kept; a pre-upgrade assignment replays; plans work on pre-upgrade missions and refuse the accepted and cancelled ones) |
| Pure rules shared with Kotlin | `TestSharedTripMissionVectors` and `EnterpriseTripMissionVectorsTest` over the same 317-case oracle |

## Validation

Final gates ran on the merged tree: the coordinator moved `codex/enterprise-f4`
twice while this task ran (to `8ce567b5`, with EMP-CHECK-003 and its
migration 000230, then to `bff5a788`, with EMP-TRIP-005); both were merged
by fast-forward under this uncommitted task, the four and three shared-file
conflicts (contract, EDR table, trip oracle README, route counts, the
cost-centre fixture copy) resolved as unions, the 000231 usage bodies rebuilt
from the 000230 bodies, and every gate below rerun. An earlier union on
`95c0e389` (`trip009-db-union-premerge-aborted.jsonl`, 901 top-level / 8,658
subtests, 0 failures) and one on `8ce567b5` (`trip009-db-union-8ce567b5-aborted.jsonl`,
538 / 8,203, 0 failures) were stopped when the heads moved and are not
claimed.

Environment: worktree backend at `bff5a788` plus this task, Go 1.27.4
toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; the
coordinator MinIO (`127.0.0.1:32819`, bucket `validation-private`) for the
S3-backed suites; Kotlin through the shared Gradle flock with JDK 17.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean; `GOTOOLCHAIN=go1.27.1 staticcheck` on trip,
  organization, enterprisesuite, apicontract, cmd/api and
  `db/queries/mission`, and with `-tags=integration` on trip,
  organization/adapters and orgtest: no findings (`trip009-staticcheck.log`).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml` on the merged
  tree: 0 errors, 1,060 warnings / 660 informs vs 1,057 / 660 on the F4 head
  `8ce567b5` (`trip009-merged-vacuum*.log`; `bff5a788` adds only response
  fields); the patience diff against the F4 head is purely additive (+225
  lines).
- Route counts on the merged tree: 146 normal routes (routed 28 / not routed
  56), 25 special; AUTH-011 auditor walk 146 routes / 30 positive / 116
  FORBIDDEN; authorization matrix 280 cells (112 allowed / 168 denied); 8
  signed operation owners (before the merge: 142, 26/58, 142/30/112, 260
  cells, 7 owners).
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'`: 3,541
  tests, 0 failures, errors or skips (`trip009-merged2-kotlin.log`; 3,374 on
  `95c0e389` before the merges).
- Go trip packages under race: all pass; `cmd/api` 22/22 top-level on the
  merged tree (`trip009-merged2-cmd.jsonl`).
- Focused PostGIS rerun after the first merge (`TestTripMission*`, unit and
  cost-centre usage sources, the incident upgrade) →
  `trip009-merged-focused.jsonl`: 38 top-level / 97 subtests, 0 failures.
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=90m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/trip/... ./internal/modules/fleet/...
  ./internal/modules/orgsync/... ./internal/modules/orgaccess/...
  ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
  ./internal/platform/migrate/... ./internal/platform/orgtest/...
  ./internal/platform/orgapi/... ./cmd/api/...` (with MinIO) on the merged
  tree → `trip009-db-union.jsonl`: 24 packages (23 ok, `orgtest` has no
  tests), 1,084 top-level / 9,251 subtests passed, 0 failures, 0 skips, 24.6
  min (`organization/adapters` 1,350 s, `apicontract` 1,189 s).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `trip009-untagged-all.log`: exit 0, 223 packages ok, 81 without tests, 0
  FAIL.
- `cmd/worker` integration tests were not run (untouched; their known
  failures need pinned Rust preparer CLIs this machine lacks); nothing here
  composes a worker job.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII: synthetic
  identifiers, labels and CPF-shaped test strings that are not valid CPFs
  (`12345678909`/`123.456.789-09`, the repository's existing synthetic
  shape); the MinIO credentials live only in the log environment file (not
  committed).

## Retained

- The app display of plans in the missions inbox and planned-trip execution
  (EMP-TRIP-021) and the panel board (EMP-TRIP-022); a reference-feed entity
  for missions and plans (EMP-TRIP-021).
- Planned stops and legs versus actual (EMP-TRIP-010) and route plans with
  points and planned distance (ROUTE).
- Re-planning an accepted mission by withdrawing the acceptance (EDR-246
  alternative), title or window edits, and a plan history read route.
- Vehicle change (EMP-TRIP-013); the trip link, `START`/`COMPLETE` and the
  `IN_PROGRESS`/`COMPLETED` states (EMP-TRIP-021/023).
- Production grants (README subsection) and live composition review.
- No milestone, phase or gate is claimed.
