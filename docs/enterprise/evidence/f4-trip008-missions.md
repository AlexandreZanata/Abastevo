# EMP-TRIP-008 — Missions: create, assign, accept and decline

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task on `codex/enterprise-f4-trip008` (worktree
`/data/dev/worktrees/f4-trip008`), based on the F4 head `75c18421` and, after the
rules, the pure domain and the oracle were written, fast-forwarded to the
TRIP-004 integration head `7daf0010` (`codex/enterprise-f4-integ`) as the
coordinator asked; the contract section and EDR rows written before that were
reapplied as unions, and every other shared file was edited on that head. Real
dependencies used: TRIP-001/002 (trip domain, `TripAuthority`, the signed
intake owners), TRIP-004 (`trip/adapters/http`, `TripStructure` precedent),
FLEET-002/003/005/006/007 (assets, lifecycle, allocation, assignments, the
`StartDecisionDBTX` port), AUTH-006 structural scope, OFFL-002/009/012 intake,
the audit runner and the company HTTP entry. Nothing is faked; synthetic data
only. No Android, panel, job or deployment change.

Read: the EMP-TRIP-008 block and the TRIP-009/013/021/022/023 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3, 6, 7,
UC-TRIP-03, SCR-TRIP-03; [08-state-model](../08-state-model.md) §§1, 3, 4
(the Mission row and rule 7); [07-permissions](../07-permissions.md) §7.3
(`trip.mission.manage`, `trip.trip.start`), §8 SELF interpretations, §10;
[10-offline-sync](../10-offline-sync.md) BR-OFFL-004/006/008/009/011, §8,
§10.2 and the §10.3 rows "My assets, assignments, missions" and "Accept or
decline a mission"; REQ-07.1/07.2, BR-AUTH-001, BR-REV-001; EDR-161, 165,
177, 192..195, 202..206, 213..215, 220..223; the evidence of TRIP-001,
TRIP-002, TRIP-003, TRIP-004, FLEET-007 and CHECK-001.

IDs used: B-BR-EMP-353..358, BUC-EMP-178..180, EDR-228..231 — all of them; no
pre-allocated ID is left unused. Migration `000227_organization_trip_missions.sql`
only; every test derives the version from one file-name constant
(`missionMigration`) and builds the previous chain as `name < 000227`.

## Rules frozen before behaviour

Observed before any change: no mission code, table or route existed;
`trip.mission.manage` and `trip.trip.start` were `NOT_YET_ROUTED`; the frozen
OFFL-001 ACK vocabulary has no `CONFLICT` rejection code (only transport
denials can carry it); the Android client sends offline only through the
signed queue and reads online through the company session; the latest usage
bodies are 000223 (asset) and 000220 (unit).

Rules B-BR-EMP-353..358 and use cases BUC-EMP-178..180 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-008 — Missions: create, assign, accept and decline (2026-11-15)";
decisions EDR-228 (pure domain, table, Kotlin mirror, oracle), EDR-229 (the
response channel and its outcome mapping), EDR-230 (persistence, usage
sources, trip link retained) and EDR-231 (routes, permissions, matrix and
auditor) in [16-decisions](../16-decisions.md). They are the recommended
routine decisions, taken unattended.

Decisions worth reading first:

- **Response channel (EDR-229).** The offline matrix makes accept/decline an
  offline action, and only the signed queue is offline on Android, so the
  response is the signed `trip.mission.respond` schema 1 (entity `mission`,
  `expected_revision` required, payload exactly `response` and `reason`). It is
  decided on current facts at receipt only — never on the declared instant —
  so an "offline result" cannot bypass revalidation. The brief's `CONFLICT`
  (08-state-model §4.7) is expressible in the frozen vocabulary only as a
  registered transport denial: `FORBIDDEN` (another member or an ended
  assignment/coverage), `NOT_FOUND` (no such mission here, including another
  organization's) and `CONFLICT` (stale revision or a mission no longer
  `ASSIGNED`) store nothing, like TRIP-002's `FORBIDDEN`/`NOT_FOUND`. No held
  form (a response is a commitment about future work; a held item would have
  no resolver) and no late holder (an ended membership is decided by OFFL-009
  before the owner).
- **Trip link (EDR-230).** No `mission_id` is added to `trip.start` schema 1
  (exact payload); a schema 2 with an optional `mission_id` checked at receipt
  and the `IN_PROGRESS`/`COMPLETED` mission states it produces belong to
  EMP-TRIP-021/023. Planning or assignment never starts a trip.
- **Reads (EDR-231).** Planner reads under `trip.mission.manage` (list ORG
  only, read in structural scope); the assignee's own list under
  `trip.trip.start` on the membership itself. AUDITOR holds neither key and is
  `FORBIDDEN` on all six routes; an auditor read of missions belongs to the
  trip panel reads (EMP-TRIP-006/022).

## Shape

- Pure Go `trip/domain/mission.go` (standard library plus the existing trip
  references and the METER-001 text profile): `MissionState` (8, terminal
  COMPLETED/CANCELLED), `MissionCommand` (7), `MissionResponse`,
  `MissionTransition` (the EDR-228 table), `MissionWindow`
  (whole-microsecond, half-open), `NewMission` (DRAFT, revision 1),
  `RestoreMission` (stored invariants), `Mission.Assign/Respond/Cancel` with
  the fixed failure order, new `ErrConflict`; values redacted on fmt, JSON and
  slog.
- Kotlin mirror `domain/.../enterprise/trip/TripMission.kt`
  (`TripMission.create/restore`, `assign/respond/cancel`,
  `TripMissionLifecycle.transition`, `TripMissionFailure` — a separate closed
  failure vocabulary like `TripLogbookFailure`, so trip-v1 is untouched).
- Oracle `contracts/testdata/enterprise/trip/mission-v1.psv` (238 cases:
  STATE 20, COMMAND 15, RESPONSE 8, TRANSITION 62 — all 56 pairs, CREATE 29,
  RESTORE 34, ASSIGN 23, RESPOND 32, CANCEL 15; SHA-256
  `d591d9e496cb686e24658f59f91896892e323afaa35f1d5ba4bf2c752021c21f`) and its
  README section. It was generated by an independent Python model of the
  frozen rules (`/data/dev/worktrees/f4-logs/trip008-oracle/gen.py`, not
  committed) before any implementation; both consumers assert the digest,
  counts, unique IDs, allowed arguments, complete result maps, every
  operation, every error code and every state/command pair.
- Application `trip/application/missions.go`: `MissionPolicy` (five fixed
  policies), `MissionCommand` (CREATE/ASSIGN/CANCEL, canonical digest),
  `MissionService` (live decision before and after the repository, result
  validation), `ParseRespond`, ports `MissionAuthority`,
  `MissionResponseAuthority`, `MissionResponseStore`, `MissionRepository`.
- Migration `000227_organization_trip_missions.sql` (EDR-230): forced-RLS
  head `org_trip_missions` (shape CHECK mirroring `RestoreMission`; BEFORE
  INSERT source guard: DRAFT revision 1, ACTIVE unit, ACTIVE non-implement
  asset, current creator; BEFORE UPDATE guard: content immutable, revision +1,
  must match the already-inserted transition of the new revision; no delete),
  insert-only `org_trip_mission_transitions` (BEFORE INSERT: the EDR-228 table
  in SQL `org_trip_mission_next`, current revision, eligible ACTIVE assignee
  of an ACTIVE unmerged employee, no same-member reassignment, a response is
  the exact `trip.mission.respond` operation of the current assignee on its
  enrolled device), insert-only receipts `org_trip_mission_commands` (with the
  warning), deferred publication (CREATE transition present; company
  transitions with their receipt; responses only with their ACCEPTED ACK and
  effect), and the 000223 asset / 000220 unit usage sources with one new line
  each (associations; counts unchanged).
- sqlc package `db/queries/mission` (own stanza, owned by the trip module,
  EDR-230); store `trip/adapters/missions.go` (authority → root lock →
  receipt replay → one audit event → effect → receipt → authority; reads
  between two authority checks; the intake side takes root then mission row);
  the intake owner `trip/adapters/mission_respond.go`.
- Authority: `organization/adapters/mission_authority.go` —
  `MissionAuthority` (company: org-level create/list, mission-targeted
  read/assign/cancel with the new `MissionStructure` kind, own list on the
  membership, ACTIVE root for commands, `EligibleAssigneeTx` = the response's
  coverage rule) and `TripAuthority.CheckMissionResponseTx` (device: current
  operational device, `trip.trip.start` with the stored assignee as
  `AssignedTo`, the stored unit/asset as structure; receipts with the stored
  responder as `AuthoredBy`; root = ORG grants only); the company policy
  allow-list admits the three mission policies.
- HTTP `trip/adapters/http/missions.go` (EDR-231): six routes, strict bodies,
  composed by `enterprisesuite` with admission class `trip.mission`;
  `enterprisesuite/sync.go` registers the owner with `NO_ASYNC_FOLLOWUP` for
  ACCEPTED and REJECTED.
- Contract: six paths, 13 schemas and the `trip.mission.respond` owner with
  `EnterpriseTripMissionRespondPayload` in `contracts/openapi/v1.yaml`
  (patience diff purely additive, +761 lines); inventory: `trip.mission.manage`
  ROUTED (5 routes, representative the list), `trip.trip.start` ROUTED (the own
  list); router test paths +6; apicontract owner count 6 → 7 and
  `TestTripMissionContractOwnerShapesAndGuards`.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip008-*`. The pure domain and the
Kotlin mirror were test-first. The application, store, intake owner,
authority and HTTP adapters were drafted before their PostGIS and router
tests (the same honest deviation TRIP-002 recorded); their RED was obtained by
freezing the tests and the migration and running them against stubbed entry
points. No expectation changed between RED and GREEN.

- Go domain (compiled stubs returning `errNotImplemented` and empty
  vocabularies) `go test -race -json ./internal/modules/trip/domain/` →
  `trip008-go-domain-red.jsonl`: 18 top-level, 1 failing
  (`TestSharedTripMissionVectors`), 236/238 vectors failing; the two passing
  rows (`create-identity-malformed`, `create-asset-malformed`) are refused by
  the existing TRIP-001 scoped-reference constructor inside the fixture.
- Kotlin (`TODO()` stubs) `:domain:test --tests '…trip.EnterpriseTripMission*'`
  (flock) → `trip008-kotlin-red.log`, XML in `trip008-kotlin-red-xml/`: 238
  tests, 236 failures (the same two rows refused by `ScopedTripRef`).
- Router RED (routes wired, spec and inventory at the merged head `7daf0010`)
  `go test -json ./cmd/api/` → `trip008-cmd-red.jsonl`: 4 top-level failing
  (`TestAuthorizationInventoryMatchesRegisteredMeterDescriptor`,
  `TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff`: undocumented
  `POST /v1/orgs/{org_id}/missions/create`). Contract RED
  (`TestTripMission*` against the head spec) → `trip008-contract-red.jsonl`:
  1/1 failing (owner absent).
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration -p 2 ./internal/modules/organization/adapters -run
  '^TestTripMission'` → `trip008-db-red-schema.jsonl`: 9/9 failing (42P01;
  the upgrade test on the missing file).
- PostGIS behaviour RED (migration present; `CommandMission`,
  `ListMissions`, `ReadMission` and the owner's `Admit`/`Apply` stubbed to zero
  values) → `trip008-db-red-behaviour.jsonl`: 9/9 failing at their first
  behaviour check (no mission created, 503 on HTTP creation, no permission
  denial, no lock wait). A first behaviour-RED attempt hung: the concurrency
  test held the organization root in an administrative transaction and only
  released it on success, so the failed wait left the root locked for the
  next subtest; both lock-holding tests now release through `t.Cleanup`, and
  the RED was rerun in full. While stopping that hung run I terminated a
  process I had not verified first, which was another agent's
  (`f4-trip016`) `organization/adapters` union run (its log shows `signal:
  terminated` at 04:53:56); this was reported to the coordinator.

## GREEN and REFACTOR

- Go domain → `trip008-go-domain-green.jsonl`: 18/18 top-level and 238/238
  vectors under race at the first implementation, after one harness fix found
  by GREEN: the three `*-scope-other` command rows restored their stored
  mission in the command scope; the fixture now restores in organization A and
  acts in the requested scope (no expectation changed). Characterization
  tests added afterwards (redaction, snapshot independence, guard branches):
  `mission.go` 100.0 % statements, package 99.8 %
  (`trip008-go-domain-cover.out`).
- Kotlin → `trip008-kotlin-green.log`: 238/238 at the first implementation;
  `EnterpriseTripMissionTest` added; the whole enterprise domain suite
  `:domain:test --tests 'com.anpfuel.domain.enterprise.*'` → 3157 tests, 0
  failures, errors or skips (`trip008-kotlin-enterprise-final.log`).
- Router and contract: `cmd/api` and `TestTripMissionContractOwnerShapesAndGuards`
  pass; apicontract owner count 7.
- PostGIS focused (`-race -tags=integration`, ordinary NOSUPERUSER /
  NOBYPASSRLS runtime role) → `trip008-db-green.jsonl`: 8/9 top-level passed
  at the first run, including both concurrency tests in both orders; the
  schema test failed four rewrite cases with 42501 because the runtime role
  lacked UPDATE/DELETE on the history tables, so the privilege check refused
  before the guard. The test now grants UPDATE, DELETE and TRUNCATE (the
  TRIP-002 precedent) so the guards themselves are proven, and adds a
  TRUNCATE case → `trip008-db-green-schema.jsonl` 1/1, 17 subtests.
- Found while reviewing GREEN, fixed before the union: the response path
  locked the mission row before the organization root (its transition trigger
  then took the root), the reverse of every company command (root then
  mission); a response racing a reassignment could deadlock. `MissionTx` with
  `lock` now takes the root first; the race test passes in both orders.
- The synthetic unit-usage fixture copy (`installSyntheticUnitUsageSource`,
  the FUEL-002 trap) lists `org_trip_missions_unit_fkey`.
- Characterization (not counted as RED): application
  `TestParseRespondIsExactAndMalformedIsFinal`, `TestMissionPoliciesAreFixed`,
  `TestMissionCommandsAreExactPerKind`.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: only the assigned eligible member responds | PostGIS `TestTripMissionResponseAtReceipt` (the ADMIN member who is not the assignee: whole-response `FORBIDDEN`, nothing stored; the assignee: `ACCEPTED` at revision 3, one transition, effect, claim, receipt and audit event); `TestTripMissionResponseRevalidatedAtReceipt/reassigned-before-receipt` (the previous assignee `FORBIDDEN`, the new assignee accepted); domain RESPOND vectors (`FORBIDDEN` before `CONFLICT`); assignment eligibility: a FINANCE-only member and the AUDITOR are `VALIDATION_FAILED` (store and HTTP), an unknown employee `NOT_FOUND` |
| AC: a response does not start the vehicle or alter review state | `TestTripMissionResponseAtReceipt` compares, before and after the acceptance, the trip count, TRIP-004 review entries, reservations, blocks, sync review items and every asset row (except `updated_at`) — unchanged; the assignment warning test asserts no trip exists after assignment; migration shape: a response transition can only be `ACCEPT`/`DECLINE` and only moves the mission head |
| Wrong member (FORBIDDEN, nothing changes) | intake `another-member-forbidden`; HTTP: driver and auditor `FORBIDDEN trip.mission.manage` on list/read/cancel; SQL `company-response` and `response-without-signed-operation` 23514 |
| Duplicate acceptance: idempotent replay vs a second, different response | same operation and digest replays the stored ACK byte for byte (twice); same id with another digest `REJECTED OPERATION_ID_REUSED`; a decline or a second accept on the answered revision, and a decline on the current revision, are `CONFLICT` with nothing stored and the head unchanged; company commands replay their first result (with the warning) and reuse is `OPERATION_ID_REUSED` (store and HTTP) |
| Ended assignment / membership at response time (revalidated at the server) | `TestTripMissionResponseRevalidatedAtReceipt`: reassignment → `FORBIDDEN`; role ended (FINANCE only) → `FORBIDDEN`; membership ENDED → source denial at the intake source, nothing stored; cancelled → `CONFLICT`; each mission head unchanged |
| Offline result bypassing revalidation | every response in the revalidation test is declared while the member was the assignee and delivered after the change; the decision uses only current facts (EDR-229), never the declared instant |
| Another organization's mission → not found | intake: a response naming another organization's mission is `NOT_FOUND` with nothing stored; store: read and cancel through another organization → `ErrNotFound`, its list is empty; HTTP read through the other organization's path → 404; SQL: another scope sees 0 rows, a cross-organization insert 42501 |
| Manager outside structural scope (AUTH-006) → FORBIDDEN | store and HTTP: a UNIT-scoped FLEET_MANAGER reads and assigns a mission of its unit, is `FORBIDDEN` on the other unit's mission, on creation and on the organization list; matrix `unit-scope` variant on both representatives |
| AUDITOR → FORBIDDEN on commands (reads decided) | AUDITOR holds neither key (EDR-231): store read/cancel and HTTP list/read/mine/cancel `FORBIDDEN`; AUTH-011 walk: all six routes `FORBIDDEN` with unchanged private state |
| Revision CAS on every command | domain ASSIGN/RESPOND/CANCEL stale vectors; store: stale assignment `ErrConflict`, assignment and cancel after cancel `ErrInvalidTransition`; HTTP 409; the response's `expected_revision` is required (`MALFORMED` without it) |
| Concurrency | `TestTripMissionConcurrentAssignmentsResolveToOne` (two assignments on one revision queued behind a held root lock, both orders: the first queued wins, the other `CONFLICT`, exactly two transitions); `TestTripMissionResponseRacesReassignment` (a response and a reassignment queued behind a held mission row, both orders: response-first → ACCEPTED and the reassignment `CONFLICT`; reassignment-first → the response `FORBIDDEN`, nothing stored) |
| Audit event per transition | one `trip.mission.created/assigned/cancelled` audit event and one receipt per company command (counted); a response is audited by its intake receipt (counted) |
| Asset eligibility and warning | creation with an unknown asset `NOT_FOUND`, a DRAFT asset or an ACTIVE IMPLEMENT `VALIDATION_FAILED`, SQL guard 23514; assignment of a member not assigned to the vehicle returns `ASSET_UNAVAILABLE` / `AVAILABLE` and stores it in the receipt; the vehicle's assigned member gets no warning |
| History, guards, RLS | `TestTripMissionSchemaIsForcedRLSGuardedAndAppendOnly` (forced RLS on 3 tables; head update without transition, content change, delete → 55000; transition and receipt update/delete → 55000; born-assigned head, implement asset, missing CREATE transition, reversed window, skipped revision, same-member reassignment → 23514); full transition history `CREATE,ASSIGN,ASSIGN,CANCEL` kept |
| Populated upgrade with failure and recovery | `TestTripMissionPopulatedUpgradeFailureRecoveryAndPreviousImage` (previous image `< 000227` containing 000225 and, on this head, 000226; completed and in-progress trips through the real intake; injected failure leaves schema, usage functions and data untouched; ordinary migrator with only REFERENCES and function ownership; one recovery, no-op replay, previous image compatible; usage answers unchanged; every allow-list line kept; a mission is created, assigned and accepted on the upgraded schema) |
| Pure rules shared with Kotlin | `TestSharedTripMissionVectors` and `EnterpriseTripMissionVectorsTest` over the same 238-case oracle |

## Validation

Environment: worktree backend on the merged head `7daf0010` plus this task,
Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; the
coordinator MinIO (`127.0.0.1:32819`, bucket `validation-private`) for the
S3-backed suites; Kotlin through the shared Gradle flock with JDK 17.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean (the new `db/queries/mission` package is committed);
  `GOTOOLCHAIN=go1.27.1 staticcheck` on trip, organization, enterprisesuite,
  apicontract, cmd/api and `db/queries/mission`, and with `-tags=integration`
  on trip, organization/adapters and orgtest: no findings
  (`trip008-static.log`).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  1,036 warnings / 660 informs vs 1,018 / 660 at the merged head
  (`trip008-vacuum.log`); the patience diff is purely additive (+761 lines).
- Route counts on this tree: 140 normal routes (routed 26 / not routed 58),
  25 special; AUTH-011 auditor walk 140 routes / 30 positive / 110 FORBIDDEN;
  authorization matrix 260 cells (102 allowed / 158 denied); 7 signed
  operation owners.
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'`: 3157
  tests, 0 failures, errors or skips.
- Go trip packages under race (`trip008-go-trip-green.jsonl`): 4 packages,
  46 top-level / 651 subtests.
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=60m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/trip/... ./internal/modules/fleet/...
  ./internal/modules/orgsync/... ./internal/modules/orgaccess/...
  ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
  ./internal/platform/migrate/... ./internal/platform/orgtest/...
  ./internal/platform/orgapi/... ./cmd/api/...` (with MinIO) →
  `trip008-db-union.jsonl`: 24 packages (23 ok, `orgtest` has no tests),
  1,041 top-level / 8,597 subtests passed, 0 failures, 0 skips, 24 min
  (`organization/adapters` 1,293 s, `apicontract` 1,121 s).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...` →
  `trip008-untagged-all.log`: exit 0, 222 packages ok, 81 without tests, 0
  FAIL.
- `cmd/worker` integration tests were not run (untouched; their known
  failures need pinned Rust preparer CLIs this machine lacks); nothing here
  composes a worker job.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII: synthetic identifiers,
  labels and fixed synthetic test cursors only; the MinIO credentials live
  only in the log environment file (not committed).

## Retained

- The trip link: `trip.start` schema 2 with an optional `mission_id` checked
  at receipt, `START`/`COMPLETE` and the `IN_PROGRESS`/`COMPLETED` states
  (EMP-TRIP-021/023); reservation linkage and `trip.mission.manage`-only
  reservation requests (EMP-TRIP-023, EDR-195).
- `PLAN` (DRAFT → PLANNED) and planning edits — destination, reason,
  customer or project, cargo or passengers (EMP-TRIP-009); vehicle change
  (EMP-TRIP-013).
- The app inbox, the reference-feed mission entity and the device handling of
  a whole-batch `CONFLICT`/`FORBIDDEN` transport denial (EMP-TRIP-021); the
  panel board with filters and history (EMP-TRIP-022).
- Auditor and trip-panel reads of missions under `trip.trip.read`
  (EMP-TRIP-006/022); notifications of assignment and response (NOTIF).
- A held or late form of a response, if the product later wants late answers
  reviewed (EDR-229 alternative).
- Production grants (README subsection) and live composition review.
- No milestone, phase or gate is claimed.
