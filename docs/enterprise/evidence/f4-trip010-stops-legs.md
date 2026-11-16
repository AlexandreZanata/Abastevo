# EMP-TRIP-010 — Stops and legs: planned versus actual

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 DB task (module TRIP, P1, size M) on
`codex/enterprise-f4-trip010` (worktree `/data/dev/worktrees/f4-trip010`),
built on the F4 integration head `c68f3537` (TRIP-019 and CHECK-004
included). Real dependencies used: the EMP-TRIP-003 typed logbook events and
`TimelineTx` order, the EMP-TRIP-008/009 mission store, `PLAN` command and
planning revisions (000227/000231), the EMP-TRIP-023 trip-mission link
(000249), the EMP-TRIP-012 own-trips policy and authority, the EMP-TRIP-011
cancellation and EMP-TRIP-014 closure facts, the signed intake and the
company HTTP entry. Synthetic data only; no Android screen, panel, job or
deployment change; no emulator or device.

Read: AGENTS, FAST_EXECUTION, `00-conventions`; the EMP-TRIP-010 block and
the TRIP-003/006/009/021/023 blocks ([EP06](../backlog/ep06-trips.md));
[trips](../modules/trips.md) §§1–12; REQ-07.1/07.3 in
[17-traceability](../17-traceability.md); the glossary entries "Leg" and
"Route plan"; [06-domain-model](../06-domain-model.md) (Mission → planned
stops; Trip → stops, legs); BR-TIME-001..006 and BR-TRUTH-003 in
[04-business-rules](../04-business-rules.md); BR-OFFL-006/007 and the offline
matrix row "Logbook events, stops, pause, resume" of
[10-offline-sync](../10-offline-sync.md); BR-ROUTE-002/008 and UC-ROUTE-01
in [gps-navigation](../modules/gps-navigation.md); `trip.trip.start`,
`trip.trip.read` and `trip.mission.manage` in
[07-permissions](../07-permissions.md); EDR-207..209, 246..249, 255, 265,
281, 331..334; the TRIP-003, TRIP-008, TRIP-009, TRIP-012, TRIP-019 and
TRIP-023 evidence; `AGENT_BRIEF.md` and `HEAD_UPDATE.md`.

IDs used: B-BR-EMP-475..480, BUC-EMP-242..244, EDR-307..310 — all of them;
no pre-allocated ID is left unused. Migration
`000256_organization_trip_mission_plan_stops.sql` only; every test derives
it from one file-name constant (`tripStopsMigration`).

## Rules frozen before behaviour

Rules B-BR-EMP-475..480 and use cases BUC-EMP-242..244 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-010 — Stops and legs: planned versus actual (2026-11-16)";
decisions EDR-307 (planned stops on `PLAN`), EDR-308 (observed stops and
legs derived, never stored), EDR-309 (the own-trip read route) and EDR-310
(migration 000256) are in [16-decisions](../16-decisions.md). They are the
recommended routine decisions, taken unattended (owner standing decision).

Decisions worth reading first:

- **Smallest design (EDR-307/308).** Planned stops ride on the existing
  `PLAN` command (revision check, receipt, audit, freeze at acceptance) as an
  optional 1..20 list of free-text places; a plan without stops keeps the
  `trip.mission.plan.v1` digest byte for byte, stops are bound by
  `trip.mission.plan.v2`. The observed side is derived at read time from the
  EMP-TRIP-003 `STOP`/`ARRIVAL`/`DEPARTURE` events — no new signed
  operation, no stored actual row, no soft reference pairing an observation
  with a planned stop, no Android producer change. `mission-v3.psv` and the
  mission plan value stay unchanged; the new pure rules (planned-stop value,
  planned legs, observed derivation) are shared by Go and Kotlin through the
  new `trip/stops-v1.psv`.
- **Text-only stops (EDR-307).** No unit, coordinate, time, person or
  contact: no new unit association, so no usage source is redefined (the
  latest 000244 bodies are untouched and the upgrade test proves it); route
  plans with typed points and planned distance remain ROUTE.
- **Incoherence surfaced (B-BR-EMP-477).** Unpaired departures and
  arrivals become explicit `ARRIVAL_MISSING`/`DEPARTURE_MISSING` boundaries
  (never paired by count, receipt or kind); a boundary declared after its own
  receipt is flagged `after_receipt`; an event accepted while the trip was
  open but declared after the end recorded later is listed apart as
  `after_end`; held stop observations are listed with their code.
- **Read route (EDR-309).** `POST /v1/orgs/{org_id}/trips/mine/{trip_id}/stops/read`
  under the EMP-TRIP-012 own-trips policy (`trip.trip.start` over the
  requesting membership); own = the starting member or the current
  responsible member; everything else is the same `NOT_FOUND`.
  `trip.mission.manage` never substitutes for a trip read; the panel trip
  detail stays EMP-TRIP-006 (`trip.trip.read`).

Rule revision before commit (recorded, not hidden): the first version of
B-BR-EMP-477, of EDR-308 and of `stops-v1.psv` (52 rows, SHA-256
`bcf462de…208c`) refused any fact after the end as invalid input. Writing
the PostGIS incoherence test showed that the database admits a finish
declared before an event accepted earlier (no guard orders a finish after
existing events), so the read would have failed and hidden every
observation of such a trip. The rule, the EDR, the independent oracle model
and both implementations were revised to list those facts apart
(`after_end`); the revised oracle (54 rows) was first run against the
unrevised implementations (RED below). The application refusal case
`event-after-end` became the positive `after_end` case and was replaced by
`event-before-start`; no other expectation changed.

## Shape

- Pure Go `trip/domain/stops.go`: `MaxPlannedStops`, `PlannedStops`
  (`NewPlannedStops`, `Places`, `Len`, `Legs`), `PlannedPoint`/`PlannedLeg`,
  `StopMark`, `StopFact`, `StopIssue` (`ArrivalMissing`,
  `DepartureMissing`), `ObservedStop`, `ObservedLeg`, `Observation`
  (`AfterEnd`), `ObserveStops`.
- Kotlin mirror `domain/.../trip/TripStops.kt`: `TripPlannedStops`,
  `TripPlannedLeg`/`TripPlannedPoint`, `TripStopMark` (with `parse`),
  `TripStopIssue`, `TripStopFact`, `TripObservedStop`, `TripObservedLeg`,
  `TripStopObservation`, `TripStops.observe`, `TripStopsException`.
- Oracle `contracts/testdata/enterprise/trip/stops-v1.psv` (54 rows:
  PLANNED 21, OBSERVE 33; SHA-256
  `128d45271fef1b7f8b5c53003fdc96a74f6cd506d68f6869eea7d53d01105ae5`) and its
  README section, generated from an independent Python model of the frozen
  rules (`/data/dev/worktrees/f4-logs/trip010-oracle/gen.py`, not committed;
  `gen-v0.py` is the first version) before the implementations.
- Application `trip/application/missions.go`: `MissionPlanFields.Stops`
  (validated by `NewPlannedStops`, copied, digest v2 only when present);
  `trip/application/stops.go`: `StopsPolicy` (= the own-trips policy),
  `ObservedFact` (`AfterReceipt`), `ObservedStopView`, `ObservedLegView`,
  `HeldObservation`, `PlannedRecord`, `PlannedView`, `StopsRecord`,
  `TripStopsView`, `StopsRepository`, `StopsService.Read`, `BuildTripStops`.
- Adapters: `trip/adapters/missions.go` writes the stops after the head
  moves and before the receipt and attaches them to every plan read;
  `trip/adapters/stops.go` `StopsStore.ReadOwnTripStops` (own-trips
  authority before and after, own predicate, trip, accepted and held stop
  events, the linked mission's current plan and stops) — raw parametrized
  statements, the mission and trip sqlc stanzas unchanged.
- HTTP: `trip/adapters/http/missions.go` (closed `stops` items, the plan
  projection's `stops` only when present) and `trip/adapters/http/stops.go`
  (`TripStopsPath`, `RegisterStops` with `HandoverRecords` and the
  own-missions resolver), composed in `enterprisesuite/routes.go`.
- Migration `000256_organization_trip_mission_plan_stops.sql` (EDR-310).
- Contract: one path, the plan request/projection `stops`, eleven schemas
  (patience diff purely additive); inventory `trip.trip.start` three routes;
  router test path; auditor walk counts; `TestTripStopsContractShapesAndGuards`.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip010-*`.

- Go domain (compiled stubs returning zero values) `go test -race -json
  ./internal/modules/trip/domain/` → `trip010-go-domain-red.jsonl`: 3/40
  top-level failing (the three new tests), 52/52 vectors failing.
- Kotlin (`TODO()` stubs) `:domain:test --tests '…EnterpriseTripStops*'`
  (flock) → `trip010-kotlin-red.log`, XML in `trip010-kotlin-red-xml/`: 53
  tests, 53 failures.
- Oracle revision RED (54-row oracle against the unrevised implementations):
  Go → `trip010-go-domain-red2.jsonl`: 3/54 vectors failing (the three
  after-end rows). Kotlin → `trip010-kotlin-red2.log`, XML in
  `trip010-kotlin-red2-xml/`: 55 tests, 4 failures — the three after-end rows
  and `observe-note-is-not-a-stop-fact`, which failed for a harness reason
  (the fixture used `TripStopMark.valueOf`, whose `IllegalArgumentException`
  is outside the shared failure vocabulary); fixed by the domain parse
  `TripStopMark.parse` (exact tokens, else `TripStopsException`), no
  expectation changed. The first Kotlin GREEN run had queued behind other
  agents' builds and started only after this revision, so it is this RED.
- Application: the digest/validation and the stops service were drafted
  before their tests (the TRIP-009 deviation); the RED was taken with the v2
  digest branch and the stop validation stubbed out →
  `trip010-app-red.jsonl`: 4/50 top-level failing (the four new tests). A
  first run had `TestBuildTripStopsRefusesBrokenStoredInvariants` passing
  for the wrong reason (the stub refused everything); a precondition that
  the valid record builds was added before the recorded run, no expectation
  changed.
- Router (route wired, head inventory and OpenAPI) `go test -json
  ./cmd/api/` → `trip010-cmd-red.jsonl`: 3/24 failing
  (`TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff`: undocumented
  `POST /v1/orgs/{org_id}/trips/mine/{trip_id}/stops/read`). Contract (head
  OpenAPI) → `trip010-contract-red.jsonl`: 3/4 failing (the new test, the
  handover guard test and the actual-routes check), the TRIP-009 plan shape
  test passing.
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration -p 2 ./internal/modules/organization/adapters -run
  '^TestTripStops'` → `trip010-db-red-schema.jsonl`: 4/6 failing
  (`unavailable` where stops are stored or read); the incoherence and scope
  tests passed because they never touch the new table. Behaviour RED
  (migration present; `ReadOwnTripStops` returning `ErrUnavailable`, stop
  writes and stop reads stubbed) → `trip010-db-red-behaviour.jsonl`: 5/6
  failing at their first behaviour check; the schema test passed (it proves
  the real migration). The upgrade test, written afterwards, failed on the
  missing migration file → `trip010-db-red-upgrade.jsonl` 1/1.

## GREEN and REFACTOR

- Go domain → `trip010-go-domain-green.jsonl`: 40/40 top-level, 54/54
  vectors under race; package statements 99.9 % (`trip010-go-domain-cover.out`).
- Application and adapters: `go test -race ./internal/modules/trip/...` all
  packages pass.
- PostGIS → `trip010-db-green.jsonl`: 6/6 top-level at the first run;
  HTTP → `trip010-db-http.jsonl` 1/1 (9 subtests) at its first run (written
  after the implementation; its refusals are the composed-router behaviour);
  upgrade → `trip010-db-upgrade.jsonl` 1/1.
- Router and contract GREEN: `cmd/api` all pass; contract
  `trip010-contract-green.jsonl` 6/6.
- REFACTOR: the mission store's now-unused `read` helper folded into
  `readTx` (every plan read needs the transaction for the stop table).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: the member distinguishes the plan from the observed stop and leg with their recorded times | PostGIS `TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes` (BUC-EMP-243: planned revision 2, three stops, four planned legs, destination; two observed stops and three legs with START/FINISH boundaries; every boundary equals its stored row: declared instant with sub-microsecond, entry zone, device offset, receipt; nothing paired); HTTP `TestTripStopsThroughComposedCompanyHTTP` (the member's read: planned and observed sides, zones, offsets, receipts, `after_receipt`, no distance or duration key) |
| Planned stops stored per revision, history kept | `TestTripStopsPlanRevisionsKeepTheirStopsAndReplay` (revision 2 two stops, revision 3 three stops, revision 4 none, revision 5 three; the mission read, the assignee's own list and the HTTP projection carry them) |
| Out-of-scope stop | `TestTripStopsOwnReadIsScopedToTheMember` (another member's trip, an unknown trip and another organization's planted trip `ErrNotFound`; an auditor denied; a third member not found; after a handover both the starter and the receiver read the identical view); HTTP: another member 404, unknown 404, non-UUID 404, another organization's path 404, auditor 403 `trip.trip.start`, a caller-chosen member field 400 |
| Duplicate transport | `TestTripStopsPlanRevisionsKeepTheirStopsAndReplay` (same `PLAN` operation and digest replays the identical ack with no new stop row; other stops or no stops under the same operation `OPERATION_ID_REUSED`); `TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes` (observation resends return the stored ACK byte for byte; the same identifier with another digest `OPERATION_ID_REUSED`; the read is identical afterwards); HTTP replay `replayed: true` and reuse 409 |
| Changed planned data overwriting actual | `TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes` (a `PLAN` while the linked mission executes and after it completed is refused `INVALID_TRANSITION` → HTTP 409 `CONFLICT`; stops, events, held events, trips, finishes and links snapshot identical; the read is deep-equal); `TestTripStopsPlanRevisionsKeepTheirStopsAndReplay` (an accepted mission cannot be re-planned; earlier revisions keep their rows); schema test (stop rows outside their `PLAN` transaction refused, update/delete/truncate refused) |
| Incoherent time hidden | `TestTripStopsSurfaceIncoherentObservations` (BUC-EMP-244: two departures without arrival, two arrivals without departure → `ARRIVAL_MISSING`/`DEPARTURE_MISSING` stops and legs; an arrival declared ten minutes after its receipt flagged `after_receipt` while the trip is open and, once a finish is declared before it, listed in `after_end` with its own instant; an arrival after the finish held `TRIP_ALREADY_CLOSED` and listed; every accepted observation appears exactly once); stops-v1 OBSERVE rows (both languages); an observation before the start stays `MALFORMED` at intake (EMP-TRIP-003) and `ObserveStops` refuses it |
| Delivery order never changes the derivation | stops-v1 (each OBSERVE row derived from the given and the reversed order); PostGIS: the departure of the first stop delivered after the next stop keeps its own receipt and its declared position |
| Concurrency | `TestTripStopsConcurrentPlansKeepOneStopList` (two plans with different stops on one revision behind the held root, both orders: the first queued wins with only its stops, the other `CONFLICT` with no plan, transition, receipt, audit or stop) |
| Forced RLS, insert-only, guards | `TestTripStopsSchemaIsForcedRLSInsertOnlyAndGuarded` (forced RLS and invisible without the setting; update, delete, truncate 55000; after the receipt, out of sequence, CPF-shaped, blank, sequence 21 → 23514; no planning row, unknown mission → 23503; another organization setting refused; a hand-made `PLAN` transaction without its receipt refused at commit with no stop left) |
| Populated upgrade with failure and recovery | `TestTripStopsPopulatedUpgradeFailureRecoveryAndPreviousImage` (image `< 000256` containing 000255, 000249, 000231; a planned, accepted, linked and observed mission and a DRAFT mission; injected failure leaves schema, the eight kept functions — usage sources included — constraints and data unchanged; ordinary migrator with `REFERENCES ON org_trip_mission_plans` only; one recovery, no-op replay, previous image compatible; the pre-upgrade plan replays and reads without stops; the pre-upgrade trip reads its plan and observation; planned stops work on the pre-upgrade DRAFT mission) |
| Wire contract | `TestTripStopsContractShapesAndGuards` (closed stop items, 1..20, no unit/coordinate/time/contact/sequence on input, the projection's stops, the view's closed shapes refusing distance, duration, pairing, invented issues, a held fact after the end; guard metadata and drift); `TestTripMissionPlanContractShapes` unchanged |
| Pure rules shared with Kotlin | `TestSharedTripStopsVectors` and `EnterpriseTripStopsVectorsTest` over the same 54-row oracle |

## Validation

Environment: worktree backend at `c68f3537` plus this task, Go 1.27.4
toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; a
throwaway MinIO (`docker compose -p f4-trip010-s3`, bucket
`validation-private`) for the S3-backed suites, removed with `down -v`
afterwards; Kotlin through the shared Gradle flock with JDK 17.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean (no query or stanza changed); `GOTOOLCHAIN=go1.27.1
  staticcheck` on trip, enterprisesuite, apicontract and cmd/api, and with
  `-tags=integration` on trip, organization/adapters, enterprisesuite and
  apicontract: no findings (`trip010-static.log`, rerun on the final tree
  `trip010-static-final.log`).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors;
  1,132 warnings / 683 informs against 1,130 / 682 on the head spec
  (`trip010-vacuum*.log`); the diff against the head spec is purely
  additive.
- Route counts: normal routes 163 → 164 (routed 30 / signed 1 / not routed
  54 unchanged); AUTH-011 auditor walk 164 routes / 33 positive / 131
  `FORBIDDEN`; authorization matrix unchanged at 300 cells (121 allowed /
  179 denied: the `trip.trip.start` representative stays the own mission
  list); signed operation owners unchanged at 20 (no signed operation
  changed).
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.*'` (flock):
  4,429 tests, 0 failures, errors or skips; the stops vectors 55/55 (54
  vectors and the unit test) (`trip010-kotlin-enterprise.log`, XML in
  `trip010-kotlin-green-xml/`).
- Focused PostGIS run (`TestTrip*`, the auditor walk, the authorization
  matrix, unit, cost-centre and checklist tests; MinIO up) →
  `trip010-db-focused.jsonl`: 154/155 top-level, 4,880/4,881 subtests. The
  one failure was caused by this task: `TestTripMissionPlanSchemaIsForcedRLSGuardedAndAppendOnly/plan-truncate`
  expected the append-only guard (55000) but a plain `TRUNCATE
  org_trip_mission_plans` is now refused earlier (0A000) because the new
  stop table references it. Following the EMP-TRIP-019 checklist precedent
  the test now grants `TRUNCATE` on `org_trip_mission_plan_stops` and runs
  `TRUNCATE org_trip_mission_plans CASCADE`, which reaches both append-only
  guards; the expected code (55000) is unchanged and the test passed alone,
  21/21 subtests (`trip010-db-plan-schema-rerun.jsonl`).
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=120m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/trip/... ./internal/modules/fleet/...
  ./internal/modules/orgsync/... ./internal/modules/orgaccess/...
  ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
  ./internal/platform/migrate/... ./internal/platform/orgtest/...
  ./internal/platform/orgapi/... ./cmd/api/...` with the throwaway MinIO, on
  the final tree after the fix → `trip010-db-union.jsonl`: 24 packages (23
  ok, `orgtest` has no tests), 1,269 top-level / 10,050 subtests passed, 0
  failures, 0 skips, 31.7 min (`organization/adapters` 1,898 s,
  `apicontract` 1,530 s).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `trip010-untagged-all.log`: exit 0, 230 packages ok, 86 without tests,
  0 FAIL (it ran concurrently with the focused PostGIS run; the later fix
  touched only an integration-tagged test).
- `cmd/worker`, `directory` and `quality` integration tests were not run
  (untouched; their known failures need pinned Rust preparer CLIs this
  machine lacks); nothing here composes a worker job.
- The throwaway MinIO (`f4-trip010-s3`) was removed with `docker compose …
  down -v` (no container or volume left).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; the staged diff was reviewed for secrets and personal
  data: synthetic identifiers, Portuguese sample places and the
  repository's synthetic CPF-shaped strings (`12345678909`,
  `123.456.789-09`, not valid CPFs); the MinIO credentials are the public
  local-validation values and live only in the log environment file.

## Retained

- The panel (EMP-TRIP-006, `trip.trip.read`) and Android (EMP-TRIP-005/021)
  presentations of planned versus observed; a supervisor's planned-versus-
  observed read across members.
- Unit or other typed places as planned stops, planned times per stop and
  route plans with points and planned distance (ROUTE, BR-ROUTE-002);
  pairing an observation with a planned stop (a `planned_stop` soft
  reference, EDR-308 alternative).
- The review resolution state of held observations (EMP-TRIP-007) and late
  events after approval (EMP-TRIP-015); a server clock-skew estimate
  (BR-TIME-006 `CLOCK_SKEW` warnings) beside `after_receipt`.
- A guard ordering a finish after the trip's accepted events (today such
  events are listed in `after_end`).
- Production grants (backend README); no panel, Android, job or deployment
  change; no M4, F4 or gate is claimed.
