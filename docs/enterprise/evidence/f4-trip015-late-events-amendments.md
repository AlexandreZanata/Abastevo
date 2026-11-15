# EMP-TRIP-015 — Late and duplicate events, and amendments after closure

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 task (module TRIP, P1, type API, size M) in worktree branch
`codex/enterprise-f4-trip016`, committed after EMP-TRIP-016 on the F4
integration head `7daf0010`. It closes the trip review loop: the author's
resubmission after a return, late and duplicate field facts kept once as
held facts instead of silently joining a submitted or approved revision,
and the amendment of an approved trip as a new reviewed revision through
the orgamend kernel, whose approval supersedes the corrected readings
through the METER-004 path. Synthetic data only; no Android, panel, job or
deployment change.

Sources read: the EMP-TRIP-015 block and the TRIP-003/004/007/016 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3–8 and
§10 (184 610 000 → 184 600 000 m, 290 000 → 280 000 m); BR-AMD-001..006,
BR-REV-002/003/005/007, BR-TIME-004, BR-TRIP-003
([04-business-rules](../04-business-rules.md)); BR-OFFL-004/005/006/009, the
ACK table rows `STALE_REVISION`/`TRIP_ALREADY_CLOSED`, §8 "Edits" and the
offline matrix rows "Correct a returned record", "Meter reading" and
"Approve, return, reject" ([10-offline-sync](../10-offline-sync.md));
VAL-01 step 6 and VAL-26 ([15-validation](../15-validation.md)); REQ-07.5/
07.6; the `meter.reading.correct` and `trip.trip.review` catalog cells
([07-permissions](../07-permissions.md)); the PLAT-016 kernel
([evidence](f3-plat016-amendment-kernel.md), `amendment-v1.psv`, decision
order, `Decide`/`Resolve`), APPR-006 ([evidence](f3-appr006-period-lock.md)),
METER-004 (B-BR-EMP-286..290, `AMENDMENT_REQUIRED`), the TRIP-002/003/004/016
evidence and EDR-137/138, EDR-177, EDR-187..189, EDR-207..209, EDR-220..223,
EDR-232/233.

IDs used: B-BR-EMP-363..368, BUC-EMP-183..185, EDR-234..237 (all of them).
Migration `000229_organization_trip_amendments.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-363..368 and BUC-EMP-183..185 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-015 — Late and duplicate events, and amendments after closure
(2026-11-15)"; decisions EDR-234 (revisions and resubmission), EDR-235 (late
and duplicate facts held `STALE_REVISION`), EDR-236 (the amendment command
and its kernel binding, server-only) and EDR-237 (the approved amendment's
effect, derived usage, the METER-004 parent set) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Design answers the task asked to decide:

- **Revisions.** Review revisions are record revisions. The stored history
  is read as the field segment (`SUBMIT`, `RESUBMIT` and their APPR-001
  decisions) plus one segment per `AMEND` entry, each replayed by the
  unchanged APPR-001 lifecycle at absolute revision numbers; the effective
  revision is the latest approved one. No change to `review/domain` or its
  shared vectors.
- **Resubmission (held by TRIP-004 "until TRIP-015").** The existing signed
  `trip.submit` naming the returned field revision appends `RESUBMIT`
  (revision + 1, ACK entity revision = the new revision). Logbook events are
  ordinary edits only while the case is `DRAFT` or a returned field
  revision (BR-AMD-006).
- **Late and duplicate facts.** An event inside the trip after its
  submission, decision or amendment, and a `trip.submit` naming a revision
  that is no longer the current draft or returned field revision, are kept
  as OFFL-008 held facts `STALE_REVISION` (frozen ACK vocabulary,
  10-offline-sync §8); a resend replays, so a repeat has one effect; the
  database refuses a direct logbook fact on such a revision. Resolution of
  held items stays with EMP-TRIP-007; changed rights on an event or a
  submission still store nothing (`RETRY`/`UNAVAILABLE`), because authority
  holds belong to that owner too.
- **Amendments.** A company route under `meter.reading.correct` (trips §7:
  meter corrections use their owning permission; catalog v5 has no
  `trip.trip.amend`) corrects the start and/or end reading of the effective
  revision with a reason. The orgamend kernel decides `AMEND`/`EDIT` and the
  period; the next revision is `SUBMITTED` and reviewed on the existing
  decide route by someone who is neither the trip's author, its driver nor
  the amendment's author. Approval goes through `Resolve` and supersedes the
  corrected readings in the same transaction through a new fleet port
  (`MeterStore.SupersedeApprovedTx`, the METER-004 supersession without its
  own authority, audit event or approved-parent refusal).
- **METER-004 consumer.** The METER-004 approved-parent port is now a set
  (fuelings and trips): a start, finish or amendment reading of an approved
  trip is `AMENDMENT_REQUIRED`, so an approved trip's usage never changes in
  place (before this task METER-004 could supersede it directly).
- **Kotlin.** No mirror and no new oracle (EDR-236): the device only queues
  `trip.submit` (a resubmission is the same operation naming the returned
  revision) and `trip.event.record`; amendments and their review are online
  company commands, and the kernel's 193 shared vectors already are the
  single oracle for any future client.
- **Recalculation provenance.** Usage is derived per named revision with the
  TRIP-001 rule, never stored; the decision context names the effective
  revision with its usage and every amendment with its corrections, state
  and usage. No recalculation job is added: no indicator, budget or
  interval consumer of trip usage exists yet (METER-006/KPI retained).

## Implementation

- Migration `000229`: widens `org_trip_review_entries` (kinds `RESUBMIT`,
  `AMEND`; reason policy; one submission-producing entry per revision);
  insert-only, forced-RLS `org_trip_amendments` (intent, base, per point the
  meter, the reading of the effective revision with its value, the corrected
  value and the pre-allocated superseding reading; composite keys to the
  trip, the actor membership and employee, and the corrected readings) with
  its BEFORE guard (lineage, current readings and values via
  `org_trip_point_reading`, active author, period lock) and deferred
  publication; insert-only, forced-RLS `org_trip_held_submissions` with its
  guard (author source, stale condition) and the OFFL-008 completion
  trigger; `org_trip_revision_closed` and `org_trip_superseded` helpers; and
  `CREATE OR REPLACE` of `org_trip_review_entry_source()` (from the 228
  body), `org_trip_review_published()` (226), `org_trip_event_source()`,
  `org_trip_held_event_source()` and `org_sync_review_source_valid()` (225),
  each keeping every previous line and adding only the TRIP-015 clauses. No
  asset, unit or cost-centre key, so no usage source changes. sqlc: new
  `db/queries/trip/amendments.sql`; the trip stanza lists 000216 and 000229.
- Application: `trip/application/revisions.go` (`TripRevisions`,
  `SubmitKind`, `SubmitEntry`, `DecideRevision`, `AmendmentCommand`,
  `ProposeAmendment`/`ResolveAmendment` on the orgamend kernel, `Values`,
  `UsageOf`, `AmendPolicy`), `review.go` (the superseded `ReviewCase`,
  `SubmitGate`, `Submit` and `Decide` replaced, `ReviewView` additions,
  `ReviewService.Propose`), `events.go` (`EventStore.ReviewEntriesTx`).
- Adapters: `submit.go` (`SUBMIT`/`RESUBMIT`, held stale submissions,
  `ACCEPTED_FOR_REVIEW` terminal, proof), `submit_holds.go` (held
  submissions, `SubmitProof`, `Store.ApprovedParentTx`), `events.go` (the
  `STALE_REVISION` hold), `review_store.go` (revision-aware decision with
  `Resolve` and supersession, `Amenders`/`Readings` ports, the
  `trip.trip.amended` audit action, the read additions), `amend_store.go`
  (`ProposeTripAmendment`, derived usage), `http/review.go` (the propose
  route, read projection, refusal mapping); fleet
  `MeterStore.SupersedeApprovedTx`, `MeterParentSet`,
  `fleetapp.AmendmentSupersession`; organization `TripReviewAuthority.
  CheckTripAmendTx` and `TripStructure` for `meter.reading.correct`.
- Runtime: the suite composes the amendment ports, the parent set and the
  `ACCEPTED_FOR_REVIEW`/`ATOMIC_REVIEW` plan of `trip.submit`.
- Contract and inventories: OpenAPI path `…/trips/{trip_id}/amendments/
  propose` and seven schemas, entry kinds, read additions, the
  `trip.submit` owner results and descriptions; route inventory (the route
  under `meter.reading.correct`: 135 normal routes, routed 24 / not routed
  60 / specials 25), full-router list, AUDITOR walk 135 / 30 / 105, matrix
  private snapshot with `org_trip_amendments` (240 cells unchanged).

Shared trip files touched (TRIP-008 and the Android agent work nearby):
`trip/adapters/{submit.go,events.go,review_store.go,http/review.go}`,
`trip/application/{review.go,events.go}`, the TRIP-004 and TRIP-003 tests
(`trip_review_integration_test.go`, `trip_event_integration_test.go`,
`trip_event_upgrade_integration_test.go`, adapter and application unit
tests), `meter_correction_http_integration_test.go` (grant), the matrix
and auditor tests (snapshot, count), `cmd/api` tests,
`organization/adapters/{trip_review_authority.go,resource_structure.go}`,
`fleet/{application,adapters}/meter_corrections.go`,
`enterprisesuite/{routes.go,sync.go}`, `sqlc.yaml`, OpenAPI and the
inventory. Untouched: `handlers.go`, `intake.go`, `store.go`, `trips.go`,
`codec.go`, `trips.sql`, `events.sql`, `reviews.sql`.

Behaviour changes to earlier tests, by design and recorded here: TRIP-004's
"a returned case stays RETRY" and "a second or stale submission stores
nothing" now assert the resubmission and the held `STALE_REVISION`; the
TRIP-003 event upgrade test applies the remaining chain before exercising
the current owners (they read the 226+ review history); fixtures gained the
runtime grants listed below.

## RED

Shape first: rules and EDRs, then the migration, exported names, routes,
contract and the tests. As in EMP-TRIP-002/004, the application and adapter
bodies were drafted before the tests (an honest deviation from strict
test-first order); RED was obtained with the frozen tests against stubbed
bodies (`/data/dev/worktrees/f4-logs/trip015-scripts/red_stubs.py`,
applied and reverted, no stub remains): `TripRevisions`, `SubmitKind`,
`SubmitEntry`, `DecideRevision`, `NewAmendmentCommand`, `ProposeAmendment`,
`ResolveAmendment`, `UsageOf`, `ProposeTripAmendment`, `HoldSubmitTx`,
`ApprovedParentTx` and `SupersedeApprovedTx`.

- Unit (`go test -json -race -count=1 ./internal/modules/trip/...
  ./internal/modules/fleet/...`), log `trip015-go-red-unit.jsonl`: 7
  top-level fail (the five new revision/amendment tests and the two
  rewritten review tests), 202 pass (shape and unrelated tests).
- PostGIS (`-run '^(TestTripAmendment|TestTripLateAndDuplicate|
  TestTripDuplicateSubmissions)'`), log `trip015-go-red-integration.jsonl`:
  7/7 top-level and 4/4 subtests fail, all behavioural (the stubbed
  revisions leave every submission `UNAVAILABLE`).

## GREEN and REFACTOR

- First focused PostGIS run (`trip015-go-green-integration-1.jsonl`): 12
  failures, all diagnosed without changing an expectation: `entryOf` parsed
  the stored `AMEND` kind with the APPR-001 parser (fixed: the trip history
  kind is admitted explicitly), and fixtures lacked runtime grants the new
  owners need (the logbook owner reads the review history; the METER-004
  composed route's parent set reads trips; the amendment fixture's parent
  set reads fuelings); the TRIP-003 upgrade test now applies the remaining
  chain first.
- Second run (`trip015-go-green-integration-2.jsonl`): 18/18 top-level and
  27 subtests pass (TRIP-015, TRIP-003 and METER-004 suites).
- Unit: trip and fleet packages green under `-race`.
- REFACTOR: toolchain gofmt; staticcheck clean (plain and integration).

## Named negatives and acceptance → tests

| Requirement | Test(s) (real PostGIS unless noted) |
|---|---|
| AC: a repeat has one effect | `TestTripLateAndDuplicateFactsAreHeldOnceAndTheReturnIsResubmitted` (held events and submissions resent: byte-identical ACKs, one held row and one review item each; a duplicate resubmission never a second `RESUBMIT`); `TestTripDuplicateSubmissionsRaceLeaveOneSubmit/{first-wins,second-wins}` (two operations of one draft revision with a lock-wait barrier: one `SUBMIT`, one held); TRIP-004 submit test (duplicate and stale submissions held once) |
| AC: the old revision stays effective until review accepts the amendment | `TestTripAmendmentIsAReviewedRevisionVAL26` (290 000 m and an untouched meter series while revision 2 is pending; 280 000 m only after the independent approval; returned, edited and rejected amendments keep revision 2 in effect); HTTP test (`effective_usage` 290000 → 280000) |
| Late fact dropped | late-facts test: an event inside the trip while `SUBMITTED` and after approval is held `STALE_REVISION` (kept, never in the logbook, the approved trip row and history unchanged); one after the finish keeps `TRIP_ALREADY_CLOSED`; the database refuses a direct fact on the approved revision (`trip revision closed`); upgrade test holds a late event on a pre-existing submitted trip |
| Duplicate counted | the two repeat tests above; concurrent proposals leave one amendment (`TestTripAmendmentProposalsRaceInBothOrders/{end-first,start-first}`, the loser `CONFLICT`) |
| Stale decision | VAL-26 test (a decision naming the approved revision 1 after the amendment exists is `CONFLICT`; a stale proposal `CONFLICT`); late-facts test (a decision naming the returned revision after the resubmission is `CONFLICT`); unit `DecideRevision` cases |
| Recalculation loses revision provenance | VAL-26 test (`EffectiveUsage.Revision`, each amendment's `Usage.Revision`, the original reading `VOIDED` with cause `SUPERSEDED`, its link, the decision operation and the amendment reason, and kept); unit `TestRevisionUsageNamesItsRevision` (values per revision, a decreasing correction `NOT_MEASURED`, rejected amendments never leak, stored values never rewritten) |
| Self-approval of an amendment (BR-REV-003) | VAL-26 test (the manager approving its own amendment `SELF_APPROVAL`); HTTP 409; schema test (direct opening by the amendment's author refused) |
| Amendment refusals | VAL-26 test (`AMENDMENT_PENDING`, `CONFLICT`, `PERIOD_LOCKED` then admitted after the reopening, replay and `OPERATION_ID_REUSED`); HTTP test (AUDITOR `FORBIDDEN meter.reading.correct`, `MALFORMED` value, `VALIDATION_FAILED` for a missing reason, an unchanged value or another meter kind, `CONFLICT`, `NOT_FOUND`, `PERIOD_LOCKED`) |
| Readings of an approved trip need an amendment (B-BR-EMP-368) | VAL-26 test: METER-004 `SUPERSEDE` of the start, finish and superseding readings of the approved trip is `AMENDMENT_REQUIRED` through the production parent set |
| Database guards | `TestTripAmendmentSchemaGuards` (never-approved lineage, a reading the point does not use, a forged value, a skipped revision, content without its entry at commit, an `AMEND` without content, the amendment's author opening it, an approval through a defective reading port that superseded nothing refused at commit, `UPDATE`/`DELETE` `55000`, forced RLS) |
| Resubmission after return (VAL-01 step 6) | late-facts test (return, the driver's note accepted, resubmission as revision 2, replay, approval of revision 2; history keeps revision 1 and its return); TRIP-004 decide test |
| Populated upgrade from the 000228 head | `TestTripAmendmentPopulatedUpgradeFailureRecoveryAndPreviousImage` (approved, returned and submitted trips; injected failure keeps every function body, constraint and row; ordinary NOSUPERUSER migrator recovery applies exactly `000229`; no-op replay; previous image compatible; new histories owned by the forced-RLS migrator; then the returned trip is resubmitted, a late event held and the approved trip amended to 280 000 m) |
| Contract | `TestTripReviewContractOwnerShapesAndGuards` (owner results, amendment request shape, route metadata), `cmd/api` inventory (135) and full-router list |

## Validation

Toolchain Go 1.27.4 from `backend/`
(`/data/dev/worktrees/f4-logs/trip016-env.sh`), the shared PostGIS through
`orgtest`, the coordinator MinIO `127.0.0.1:32819`:

```sh
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/{trip,fleet,review,organization}/... ./internal/platform/{enterprisesuite,apicontract}/... ./cmd/api/...   # clean
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/{trip,fleet}/... ./internal/modules/organization/adapters/... ./internal/platform/apicontract/... ./cmd/api/...   # clean
sqlc vet && sqlc diff                                                 # clean
go test -json -race -tags=integration -count=1 -p 2 -timeout=90m ./internal/modules/organization/adapters ./internal/modules/{trip,review,orgamend,fleet,fueling,orgsync}/... ./internal/platform/{enterprisesuite,apicontract,migrate,orgtest,orgapi}/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root
python3 docs/enterprise/tools/check_plan.py                           # repo root
git diff --check
```

Results on the task tree (before the merge with the F4 head; the merged-tree
rerun is recorded below):

- Race/integration union, log `trip015-db-union.jsonl` (19 min 10 s;
  `organization/adapters` 1107 s): 29 packages, 28 pass and `orgtest` has
  no tests; 1008 top-level / 7528 named, 0 failures, 0 skips. It includes
  every TRIP-015, TRIP-016, TRIP-004, TRIP-003 and METER-004 suite, the
  authorization matrix at 240 cells (93 / 147, private snapshot with the
  amendments) and the AUDITOR walk at 135 routes / 30 positive / 105
  FORBIDDEN (the new route refused with `meter.reading.correct`).
- Untagged whole module, log `trip015-untagged-all.log`: exit 0, 222
  packages ok, 80 without tests.
- Static gates (`trip015-staticcheck.log`, empty): toolchain gofmt, vet plain
  and integration, staticcheck plain and integration, `sqlc vet`/`sqlc diff`
  clean.
- OpenAPI: `vacuum lint` 0 errors; 1023 warnings / 660 informs versus 1018 /
  660 on the base (five description-style warnings of the new operation and
  schemas), log `trip015-vacuum.log`.
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean; secret scan and `gitleaks` clean.
  Synthetic data only.
- No Kotlin, Android, shared vector, job or deployment change (EDR-236), so
  no Gradle run applies; no emulator or device was used.

## Merge with the integration head

Task commits `6dd09490` (EMP-TRIP-016) and `9bdd7a45` (EMP-TRIP-015) on
`7daf0010`; then, as the coordinator asked, a normal merge of
`codex/enterprise-f4` at `3345865b` (EMP-FLEET-013 and EMP-TRIP-008,
migration 000227) in merge commit `3a3eca09`. Conflicts were resolved as
unions: the contract sections (FLEET-013, TRIP-008, then TRIP-016,
TRIP-015), the EDR rows by number (224..231, then 232..237), the full-router
path list, the matrix private snapshot (missions and amendments), the
`TripStructure`/`MissionStructure` cases and the `trip.submit` and
`trip.mission.respond` intake plans; the OpenAPI file is the F4 head's with
the TRIP-016 decide description and the TRIP-015 path, schemas and owner
results re-applied structurally (one `amendments/propose` path, YAML
parses). 000227 redefines only `org_asset_current_usage` and
`org_unit_current_usage`; 000228 and 000229 redefine neither, so no usage
body had to be rebuilt, and the previous images of both upgrade tests now
include 000227. Counts on the merged tree: 141 normal routes (routed 26,
signed 1, not routed 58, specials 25), AUDITOR walk 141 / 30 / 111.

Gates on the merged tree (`3a3eca09`):

- Race/integration union with the same flags, log
  `trip015-merged-db-union.jsonl` (47 min 20 s; `organization/adapters`
  2800 s under load): 29 packages, 28 pass and `orgtest` has no tests;
  1025 top-level / 8242 named, 0 failures, 0 skips. It includes every
  TRIP-016, TRIP-015, TRIP-008, TRIP-004, TRIP-003 and METER-004 suite, the
  authorization matrix at 260 cells (102 allowed / 158 denied) and the
  AUDITOR walk at 141 routes / 30 positive / 111 FORBIDDEN with unchanged
  private state.
- Untagged whole module, log `trip015-merged-untagged-all.log`: exit 0, 222
  packages ok, 81 without tests.
- Toolchain gofmt, vet plain and integration, `sqlc vet`/`sqlc diff` and
  staticcheck plain and integration (`trip015-merged-staticcheck.log`,
  empty): clean.
- OpenAPI: `vacuum lint` 0 errors, 1041 warnings / 660 informs versus 1036 /
  660 on the F4 head `3345865b` (the five TRIP-015 description-style
  warnings), log `trip015-merged-vacuum.log`.
- `check_plan.py`: 0 problems; `git diff --check` clean.

## Retained obligations

- EMP-TRIP-007: resolution of held `STALE_REVISION` events and submissions
  (accept into an amendment, refuse with a reason); until then they change
  no revision, figure or queue. Changed rights on an event or a submission
  stay `RETRY`/`UNAVAILABLE`.
- Amendments correct only existing start/end readings; adding a reading to
  a trip that had a stated absence, correcting instants, events or the
  driver, voiding an approved trip (BR-AMD-003) and an adjustment form
  remain open (no source states them for trips yet).
- METER-004 still decides a reading of a submitted, not yet approved trip
  directly (B-BR-EMP-290 is limited to approved parents); the trip usage
  keeps the original stored value until its review (open note for
  EMP-TRIP-006/the review policy owner).
- Recalculation consumers of trip usage (indicators, budgets, findings —
  METER-006/KPI) and the `meter.usage.recalculated` outbox event for
  amendment approvals (published only when consumers exist, EDR-189).
- EMP-TRIP-005: the Android producer queues the resubmission as
  `trip.submit` naming the returned revision and shows held receipts;
  EMP-TRIP-006: the panel shows amendments and their usage.
- Production runtime grants (not provisioned here): `SELECT, INSERT ON
  org_trip_amendments, org_trip_held_submissions`; `SELECT ON
  org_trip_review_entries` for the logbook owner and guard; `SELECT ON
  org_trips, org_trip_finishes, org_trip_amendments, org_trip_review_entries`
  for the METER-004 decision; `SELECT, INSERT ON org_meter_readings,
  org_meter_reading_states` and `SELECT ON org_meters, org_meter_segments,
  company_web_sessions` for the approval's supersession; the migrator needs
  `REFERENCES ON org_trips, org_memberships, org_employee_identities,
  org_meter_readings, org_sync_review_items` and ownership of the replaced
  functions and of `org_trip_review_entries`. Proposed README text is this
  paragraph.
- No C1, M2, M4 or F4 acceptance, deployment or real data is claimed.
