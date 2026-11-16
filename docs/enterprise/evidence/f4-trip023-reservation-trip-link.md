# EMP-TRIP-023 — Reservation-to-trip integration and no-show release

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task on `codex/enterprise-f4-trip023` (worktree
`/data/dev/worktrees/f4-trip023`), first commit of the branch, based on the
F4 head `672948b8` (TRIP-012 integrated; latest trip bodies in 000243, usage
functions in 000244). Real dependencies used: the EMP-TRIP-008/009/013
mission store, authority, signed response and planning; EMP-TRIP-002/011/
012/014 trips, the signed start/finish intake, lifecycle, handover and
supervisor closure; EMP-TRIP-007 held-fact resolution; EMP-FLEET-007
reservations, the outcome job and the derived start decision; AUTH-006
structural scope; the audit runner and the company HTTP entry. Nothing is
faked; synthetic data only. No Android screen, panel, deployment or device
change.

Read: the EMP-TRIP-023 block (and TRIP-008/009/010/013/021/022 blocks) of
[EP06](../backlog/ep06-trips.md); [trips](../modules/trips.md) §§3, 6, 8;
[fleet](../modules/fleet.md) §§3, 4 (BR-FLEET-030/031), 5 (UC-FLEET-06),
6.2, 6.4, 7, 9, 12; [08-state-model](../08-state-model.md) §§3, 4;
[10-offline-sync](../10-offline-sync.md) BR-OFFL-004/007, §10.1, §10.2;
[07-permissions](../07-permissions.md) §7.3; EDR-192..195, 202..206,
228..231, 246..251, 254..256, 262..264; the f4 ledger and the evidence of
TRIP-002, TRIP-003, TRIP-005, TRIP-007, TRIP-008, TRIP-009, TRIP-011,
TRIP-012, TRIP-013, TRIP-014 and FLEET-007; the TRIP-007 agent's analysis
(`f4-logs/trip023-analysis.md`).

IDs used: B-BR-EMP-412..414, BUC-EMP-209..210, EDR-265..266 — all of them;
no pre-allocated ID is left unused. Migration
`000249_organization_trip_reservation_links.sql` (coordinator decision: the
originally reserved `000238` would run before the 000243 bodies this task
extends and stays unused). Every upgrade test derives the version from one
file-name constant (`missionReservationLinkMigration`) and builds the
previous image as `name < 000249` (it must contain 000232, 000243, 000244).

## Rules frozen before behaviour

Observed before any change: `ParseStart` answered every soft reference
`ErrUnsupported` (RETRY/UNAVAILABLE); the protocol supports only operation
schema 1 in Go and the shared Kotlin vectors; the 000243 held source admits
only `ASSET_BUSY`, `ASSET_UNAVAILABLE`, `PERMISSION_CHANGED` for held
starts; reservations (000217) had no mission column and their guard refused
`FULFILLED` ("no source before EMP-TRIP-023") while the exclusion constraint
already covered `CONFIRMED` and `FULFILLED`; the FLEET-007 job records
`NO_SHOW` once, derived at read time, with a compare-and-set; FLEET-007's
`DecideStart` admitted a start inside a confirmed window only for the
reservation's employee; the mission table (EDR-228, `mission-v2.psv`)
answered `UNSUPPORTED_COMMAND` for `START`, `COMPLETE` and every
`IN_PROGRESS` change; mission transitions only had sources `COMPANY` and
`DEVICE`; operations do not store their soft references (only their digest).

Rules B-BR-EMP-412..414 and use cases BUC-EMP-209/210 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-023 — Reservation-to-trip integration and no-show release
(2026-11-15)"; decisions EDR-265 (soft reference, ORPHAN_REFERENCE, link
table, mission table v3, trip-completion trigger, migration number) and
EDR-266 (reservation linkage, FULFILLED source, request route, start
decision, job lock order) in [16-decisions](../16-decisions.md).

Decisions worth reading first:

- **Soft reference, not schema 2 (EDR-265).** `trip.start` keeps schema 1
  and its exact payload; one envelope reference `{type: "mission", id}` names
  the mission. The Kotlin trip builders and `trip-operations-v1.psv` are
  unchanged: the device does not send the reference yet (the mission inbox is
  EMP-TRIP-021), so no byte changed and no v2 vector file was needed.
- **Held, link removed (BR-OFFL-007).** A missing, foreign or ineligible
  mission holds an otherwise accepted start `ORPHAN_REFERENCE` (no trip, no
  link); the existing held codes keep their priority; a held start never
  links (an EMP-TRIP-007 acceptance creates the declared trip without the
  mission and consumes no reservation).
- **Mission follows the trip (EDR-265).** `START`/`COMPLETE` are produced
  only by the linked trip (source `TRIP`); the completing transaction (finish
  or supervisor closure) completes the mission through an `AFTER UPDATE OF
  state` trigger on `org_trips` (the FLEET-007 same-transaction consumer
  precedent); a cancelled trip leaves the mission `IN_PROGRESS` and its
  planner may cancel it.
- **Reservation (EDR-266).** Requested by the planner through a mission
  command `RESERVE` on the mission's vehicle and planned window; consumed
  once by the linked start only at a receipt before the no-show deadline and
  with the declared start inside `[start − 30 min, end)`; the outcome job now
  takes the organization root lock first so a release and a start serialize.
  The fulfilment is an effect of the start operation (its receipt audit
  covers it). Fulfilment by a reserved-employee start without a mission is
  retained.

## Shape

- Pure Go `trip/domain/mission.go`: `MissionTransition` with the EDR-265 rows
  (`START` ACCEPTED → IN_PROGRESS, `COMPLETE` IN_PROGRESS → COMPLETED,
  `CANCEL` also from IN_PROGRESS, every other refused pair
  `INVALID_TRANSITION`); `trip/domain/mission_reserve.go` (server-only, like
  the TRIP-013 vehicle change): `ReservationRequestAllowed`,
  `Mission.Reserve`.
- Shared oracle `contracts/testdata/enterprise/trip/mission-v3.psv`
  (317 rows, 21 changed from v2; SHA-256
  `6b29bcbf3359066c553e371e1c054d2dba56632d99a1b05cc258fad45e1ef18c`)
  consumed by Go `mission_vectors_test.go` and Kotlin
  `EnterpriseTripMissionVectorsTest`; `UNSUPPORTED_COMMAND` leaves the
  mission failure vocabulary (`TripMissionFailure` and the Go error-code
  list); `TripMissionLifecycle.transition` mirrors the Go table. v2 stays in
  the tree, unconsumed.
- Fleet domain: `FulfilReservation` row CONFIRMED → FULFILLED;
  `ReservationFact.MissionID`, `Starter.MissionID`; `DecideStart` admits the
  mission's own start inside its confirmed window.
- Fleet application ports: `StartAvailabilityPort.StartDecisionDBTX` gains
  the eligible mission; `MissionReservation`, `MissionReservationPort`
  (`RequestMissionReservationTx`, `MissionReservationOfTx`),
  `MissionReservationFulfilment` (`FulfilMissionReservationDBTX`),
  `EarlyStartTolerance` (30 min).
- Fleet store `fleet/adapters/reservations.go`: the three ports; the
  availability facts carry the reservation's mission; `SettleReservation`
  takes the organization root lock first.
- sqlc `db/queries/fleet/reservations.sql` (stanza lists 000220, 000227,
  000231, 000232, 000249): `AvailabilityReservations` returns `mission_id`;
  `InsertMissionReservation`, `MissionReservationByOperation`,
  `FulfillableMissionReservation` (CONFIRMED, not yet due at
  `transaction_timestamp()`, extended window, `FOR UPDATE`),
  `FulfilReservationRow` (compare-and-set). `models.go` unchanged.
- Trip application: `MissionReferenceType`, `OrphanReference`,
  `StartInput.MissionID`; `ParseStart` admits zero or one mission reference;
  `MissionStart`, `MissionEligible`, `StartHeldCode`, `StartAvailability`;
  `Store.MissionForStartTx` / `LinkMissionTx`; held codes of a start include
  `ORPHAN_REFERENCE`; mission command kind `RESERVE` (purpose in `Reason`,
  digest form unchanged), `MissionAck.ReservationID`.
- Trip adapters: the start owner resolves and locks the named mission after
  the final refusals, passes an eligible mission to the start decision, holds
  `ORPHAN_REFERENCE` when the start would otherwise be accepted, then stores
  the link, the `START` revision and consumes the reservation in the start's
  transaction (raw parametrized SQL in `store.go`; the trip stanza does not
  include the mission tables); the mission store's `RESERVE` branch
  (transition → fleet request → head) with one
  `trip.mission.reservation_requested` audit event and replay of the
  reservation id; `missionMapped` maps the fleet window conflict
  (`RESERVATION_CONFLICT`, window only) and a started window
  (`VALIDATION_FAILED`).
- HTTP: `POST /v1/orgs/{org_id}/missions/{mission_id}/reservations/request`
  (fresh `trip.mission.manage`, `MissionStructure`, revision required, 201;
  strict body `schema_version`, `operation_id`, `revision`, `purpose`); the
  ack adds `reservation_id`. Composition: `MissionStore.Reservations` is the
  fleet asset store.
- Migration 000249 (EDR-265/266): see the contract rules. New functions
  `org_trip_mission_link_source`, `org_trip_mission_link_published`,
  `org_trip_mission_follow`; six redefinitions built by
  `f4-logs/trip023-scripts/build_000249.py`, which fails if any line of the
  latest body is lost, with the line log `f4-logs/trip023-migration-lines.log`:
  `org_asset_reservation_guard` (000217, +35 lines), `org_trip_mission_next`
  (000232, +7), `org_trip_mission_transition_source` (000231, +19),
  `org_trip_mission_guard` (000232, +7), `org_trip_mission_complete`
  (000232, +9), `org_trip_held_source` (000243, +3 and one replaced line: the
  held-start code list gains `ORPHAN_REFERENCE`). Every new subquery that
  touches another table runs only inside the branch that needs it, so
  existing principals need no new grant except `SELECT ON
  org_trip_mission_links` for trip completion. No usage function changes (no
  new key references an asset, unit or cost centre).
- Contract: one path, `EnterpriseMissionReservationRequest`,
  `EnterpriseMissionAck.reservation_id`; inventory `trip.mission.manage`
  eight routes (157 normal routes; routed 30 / signed 1 / not routed 54 /
  specials 26 unchanged); router test path +1; AUTH-011 auditor walk 157 /
  31 positive / 126 FORBIDDEN; matrix unchanged (300 cells, 121/179, no new
  representative). Backend README: production grants for 249.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip023-*`. The shared mission table,
the fleet start decision and the application rules were test-first against
the unchanged or zero-value implementations; the store, intake and HTTP
adapters were drafted before their PostGIS and router tests (the
TRIP-002/008/013 deviation) and their RED was obtained against the missing
migration and stubbed store entry points.

- Go mission oracle (v3 vectors against the v2 table) `go test -json
  ./internal/modules/trip/domain/` → `trip023-go-domain-red.jsonl`: 1
  top-level failing (`TestSharedTripMissionVectors`), 21 vector subtests
  failing — exactly the 21 changed rows; 35 other top-level tests passing.
- Go `Mission.Reserve` (zero-value stub) → `trip023-go-domain-reserve-red.jsonl`:
  1/1 failing.
- Kotlin mission oracle (v3 against the unchanged `TripMission.kt`) →
  `trip023-kotlin-red.log` (+ XML): the factory failed at its vocabulary
  assertion (`UNSUPPORTED_COMMAND` has no v3 vector) before any row ran.
- Fleet domain (fields present, unchanged `DecideStart` and FULFIL row) →
  `trip023-fleet-domain-red.jsonl`: 2 top-level failing
  (`TestReservationTransitionTableIsExactlyFleetSection64`,
  `TestStartDecisionAdmitsTheReservingMissionsStart`), 33 passing.
- Trip application (stubbed `MissionEligible`, `StartHeldCode`, held codes,
  `ParseStart` refusing every reference, `RESERVE` refused) →
  `trip023-app-red.jsonl`: 4 top-level failing
  (`TestParseStartReadsOneMissionSoftReference`,
  `TestMissionEligibleForTheStart`, `TestMissionReserveCommandShape`,
  `TestConflictViewValidationAndHeldCodes`), 38 passing.
- Router/contract (route wired, inventory, spec and walk at `HEAD`) →
  `trip023-cmd-red.jsonl`: 3 top-level failing (inventory unclassified
  route, company composition, full router "undocumented organization
  route"); `trip023-contract-red.jsonl`: 35 top-level failing (every guard
  drift test sees the undocumented route).
- PostGIS schema RED (migration moved out of the tree) and behaviour RED
  (migration present, the five store entry points stubbed:
  `MissionForStartTx` not found, `LinkMissionTx` no-op,
  `RequestMissionReservationTx` no-op, `MissionReservationOfTx` empty,
  `FulfilMissionReservationDBTX` empty): schema RED →
  `trip023-db-red-schema.jsonl`: 8/8 top-level failing (missing link table
  and columns; the upgrade test on the missing migration file); behaviour
  RED → `trip023-db-red-behaviour.jsonl`: 8/8 top-level failing (no link,
  no START, no reservation for RESERVE whose head guard then refuses, no
  consumption). Logs keep both runs; the stub script is
  `f4-logs/trip023-scripts/stub_red.py`.

Two existing expectations encoded the superseded rule and were changed with
it (no other expectation changed): the TRIP-002 codec case
`start-unsupported-soft` used a `mission` reference as its example of an
unsupported form — it now uses a `fueling` reference (still unsupported);
the TRIP-007 conflict-view case "meter code" used `ORPHAN_REFERENCE` as a
code no trip start holds — it now uses `PERIOD_LOCKED`, and the held-code
table gains the start/`ORPHAN_REFERENCE` row. The FLEET-007 table test row
`FULFIL: {}` (no source) is now `CONFIRMED → FULFILLED`, and the Go mission
error-code list drops `UNSUPPORTED_COMMAND` with the v3 oracle.

## GREEN

- Go unit (`-race`): trip domain/application/adapters and fleet
  domain/application/adapters → `trip023-unit-green.jsonl`: 232 top-level /
  1,380 subtests, 0 failures; trip domain 99.9% statements, fleet domain
  96.2%; the v3 mission oracle 317/317 in Go.
- Kotlin: trip package 954 tests, 0 failures (`trip023-kotlin-green.log`);
  enterprise domain suite under the Gradle flock 4,227 tests, 0 failures
  (`trip023-kotlin-enterprise.log`).
- `cmd/api` and `apicontract` (untagged) pass with the documented route
  (apicontract adds `TestTripMissionReservationContractShapes`).
- PostGIS focused (`-race -tags=integration`, ordinary NOSUPERUSER /
  NOBYPASSRLS runtime role) → `trip023-db-green2.jsonl` and
  `trip023-db-http.jsonl`: 8/8 top-level, 25 subtests passing:
  `TestTripMissionLinkedStartConsumesItsReservationAndCompletes`,
  `TestTripMissionReferenceOrphanAndUnsupportedForms`,
  `TestTripMissionReservationAdmitsItsOwnStartWithoutAssignment`,
  `TestTripMissionReservationReleaseRacesLinkedStart` (both orders and the
  stopped job), `TestTripMissionReserveRefusalsAndCancelInProgress`,
  `TestTripMissionReservationSchemaGuards`,
  `TestTripMissionReservationPopulatedUpgradeFailureRecoveryAndPreviousImage`,
  `TestTripMissionReservationRequestThroughComposedCompanyHTTP`.
- Found during GREEN and fixed (no expectation changed): (1) the availability
  read selected the new `mission_id` column, so every rehearsal that runs
  trip starts on a pre-249 image (TRIP-012/013 and older upgrade tests)
  would have failed; the mission ids now come from a separate query skipped
  by the rehearsal-only `AssetStore.PreMissionLinkImage` flag, which the
  `fleetStore` fixture sets from the image (the TRIP-011 `PreLifecycleImage`
  precedent; production false); (2) the start-first race initially kept the
  start waiting longer than the intake's 10-second operation bound — the
  barrier now launches the start two seconds before the deadline; (3) the
  unsupported-form and HTTP conflict assertions were corrected to the
  transport-denial and `RESERVATION_CONFLICT`-details envelopes the
  existing tests use.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| One eligible reservation → one linked use (AC) | `TestTripMissionLinkedStartConsumesItsReservationAndCompletes`: RESERVE (REQUESTED on the mission's vehicle and window, no employee, both receipts, one audit event, outcome job), fleet confirm, start naming the mission ACCEPTED, link, mission `IN_PROGRESS` (START, source TRIP, start operation), reservation `FULFILLED` with the trip; finish → mission `COMPLETED` (COMPLETE with the finish operation) |
| One eligible reservation → one recorded release (AC) | `TestTripMissionReservationReleaseRacesLinkedStart/release-before-start`: `NO_SHOW` once (one `fleet.reservation.missed`), second job run unchanged |
| Race between release and start, both orders | same test with barriers on a held organization root: start queued first (its transaction began before the deadline) → `FULFILLED`, the queued job finds it consumed and changes nothing; job queued first (after the deadline) → `NO_SHOW`, the start is accepted, linked, and never consumes it |
| Expired reservation consumed | `…/job-stopped-due-reservation-never-consumed` (stored `CONFIRMED` but due: the start does not consume it; the late job records `NO_SHOW`); `release-before-start`; SQL guard: FULFILLED at/after the deadline refused; the start-outside-early-tolerance case in the upgrade test |
| Foreign asset / assignment | `TestTripMissionReferenceOrphanAndUnsupportedForms`: unknown, other organization, cancelled, not accepted, another member's, another vehicle → `ORPHAN_REFERENCE`, nothing linked, replay byte for byte; another member's confirmed mission reservation holds the start `ASSET_UNAVAILABLE` (normal code first); `TestTripMissionReserveRefusalsAndCancelInProgress` (another mission over the same confirmed window → `RESERVATION_CONFLICT` with only the window); HTTP UNIT planner on another unit's mission 403, other organization 404 |
| Parallel active use | second start on the vehicle naming the in-progress mission → `ASSET_BUSY`, no link; one link per trip and per mission (keys); `link-of-another-member` and `link-by-another-operation` refused by the database |
| Replay / reuse | start replays byte for byte; same id with another digest `OPERATION_ID_REUSED`, no second link; RESERVE replay returns the same reservation (store and HTTP), another purpose `OPERATION_ID_REUSED` |
| Mission's own start admitted by its reservation (BR-FLEET-031) | `TestTripMissionReservationAdmitsItsOwnStartWithoutAssignment` (assignment ended: without the reference `ASSET_UNAVAILABLE`, with it accepted and `FULFILLED`); domain `TestStartDecisionAdmitsTheReservingMissionsStart` |
| Mission table v3 | Go `TestSharedTripMissionVectors` and Kotlin `EnterpriseTripMissionVectorsTest` (317 rows, same hash); planner cancel of an `IN_PROGRESS` mission, assign/reserve refused, the trip's later completion leaves it `CANCELLED` (`TestTripMissionReserveRefusalsAndCancelInProgress`) |
| RESERVE refusals | stale revision `CONFLICT`, DRAFT `INVALID_TRANSITION`/409, no vehicle `VALIDATION_FAILED`, unknown `NOT_FOUND`, open body fields `MALFORMED`, missing revision, blank purpose, driver and auditor `FORBIDDEN` (store, HTTP, contract) |
| Unsupported reference forms | another type and two references stay `RETRY`/`UNAVAILABLE` with nothing stored (PostGIS) and `ErrUnsupported` (codec) |
| Forced RLS, insert-only, database proofs | `TestTripMissionReservationSchemaGuards` (links forced RLS, invisible without the setting, update/delete/truncate 55000; FULFILLED without link, mission change, START from COMPANY, START without link, a mission reservation without its RESERVE revision refused) |
| Populated upgrade 244 → 249, failure and recovery | `TestTripMissionReservationPopulatedUpgradeFailureRecoveryAndPreviousImage` (completed and in-progress trips, held ASSET_BUSY start, ACCEPTED mission, confirmed pre-upgrade reservation; injected failure leaves schema, functions, constraints and data unchanged; ordinary migrator with REFERENCES/TRIGGER/ownership only; no-op replay; previous image check; redefined bodies keep their lines; receipts replay; the pre-upgrade trip completes with no mission; reserve → linked start → completion on the upgraded schema; remaining chain) |
| Held start never links | the start hold path stores no link (orphan and ASSET_BUSY cases); `HeldCode` admits `ORPHAN_REFERENCE` for the TRIP-007 queue; the 000237 resolution source has no code filter and creates the declared trip without a link (no new code path) |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; the shared PostGIS control
`127.0.0.1:32775` only through orgtest isolated child databases; a throwaway
MinIO project `f4-trip023-s3` from `infra/compose.validation.yml` (torn down
with `down -v` at the end).

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on trip, fleet, enterprisesuite,
  apicontract, cmd/api, db/queries/fleet, and with `-tags=integration` on
  organization/adapters, trip and fleet: no findings
  (`trip023-staticcheck.log`, empty).
- `sqlc generate` (fleet stanza with 000220/000227/000231/000232/000249) and
  `sqlc vet`, `sqlc diff` clean; only `db/queries/fleet/reservations.sql.go`
  changes (`models.go` unchanged).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  1,121 warnings vs 1,118 at `HEAD` (the existing route pattern for one new
  route); the diff is additive (one path, one schema, one ack property).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; the staged diff was reviewed for secrets/PII: synthetic
  identifiers and texts only; the disposable MinIO credentials live only in
  the log environment file (not committed).
- Kotlin (Gradle flock, JDK 17): trip package 954, enterprise domain suite
  4,227 tests, 0 failures.
- Affected race/integration union `go test -json -race -tags=integration
  -p 2 ./internal/modules/organization/adapters ./internal/modules/trip/...
  ./internal/modules/fleet/... ./internal/modules/tracking/...
  ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/...`
  (MinIO up) → `trip023-db-union.jsonl` (01:35–02:19, shared machine): 37
  packages (32 ok, 4 without tests, 1 failing), 1,313 top-level / 9,276
  subtests passing, 0 skips; the single failure was
  `TestTripClosurePopulatedUpgradeFailureRecoveryAndPreviousImage`: after
  applying the remaining chain (now including 000249) it closes a trip, and
  the completion trigger reads `org_trip_mission_links`, which that fixture
  had not granted. Fixed in the fixtures only (the conditional
  `tripMissionLinkGrant` beside `tripHandoverGrant` in the closure and
  membership-end rehearsals and the review fixture; no expectation changed;
  the README lists the grant). Rerun of every `TestTrip*` test of
  organization/adapters after the fix → `trip023-db-union-trip-rerun.jsonl`:
  104 top-level / 223 subtests passing, 0 failures (the closure rehearsal
  included).
- Untagged whole module `go test -count=1 -timeout=45m -p 2 ./...` →
  `trip023-untagged-all.log`: 229 packages ok, 87 without tests, 0 FAIL.
- `cmd/worker` integration tests were not run (outside the union; their known
  failures need pinned Rust preparer CLIs this machine lacks; untouched).

## Retained

- Android: the trip builders sending the mission reference and the
  `trip-operations` vectors for it, the mission inbox and execution
  (EMP-TRIP-021); the device does not send the reference today.
- Linking a held start when EMP-TRIP-007 accepts it (it creates the trip
  without the mission); held-start resolution of `ORPHAN_REFERENCE` beyond
  the existing accept/refuse.
- Fulfilment of a reservation by its reserved employee's start without a
  mission (fleet.md §6.4 names it; only mission reservations are consumed
  here).
- Cancelling a mission's open reservations when the mission is cancelled
  (fleet.md §6.4 system cancel) and when its vehicle changes; the fleet
  `cancel` route remains the way to release them.
- A configurable early tolerance (fixed 30 minutes) and no-show tolerance
  (60 minutes default); notifications of fulfilment and no-show.
- Vehicle change on an `IN_PROGRESS` mission (still refused); the mission
  outcome of a cancelled trip beyond the planner's cancel.
- Panel and app display of links, reservations and mission progress
  (EMP-TRIP-021/022); production grants (README); real-device acceptance.
- EMP-TRIP-010 (planned versus actual stops and legs), the second task of
  this agent's prompt, was not started: its pre-allocated rule, use-case and
  decision numbers (rules 475..480, use cases 242..244, decisions 307..310)
  and migration 000250 stay reserved and unused.
- Proposed shared text for the coordinator (not edited here):
  10-offline-sync §10.2 could name the `trip.start` mission reference as the
  first soft reference decided by an owner (`ORPHAN_REFERENCE` held start).
- No milestone, phase or gate is claimed.
