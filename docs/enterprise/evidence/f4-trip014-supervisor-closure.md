# EMP-TRIP-014 — Missing final reading and supervisor closure

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 API task (module TRIP, P1, type API, size M) in worktree branch
`codex/enterprise-f4-trip014`, opened from the F4 head `fe7ec66d` (main
`93f3a88d` + TRIP-009/013, migrations through 000232). A currently permitted
supervisor closes another member's open trip with a reason and an explicit
reason why no final reading exists; the trip completes without a finish, its
usage stays `NOT_MEASURED` and nothing is invented. Synthetic data only; no
Android, Kotlin, panel screen, job or deployment change.

Sources read: the EMP-TRIP-014 and EMP-TRIP-018 blocks and the TRIP-002/004/
007/011/012/015/016 blocks ([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md)
(UC-TRIP-03, §§6–10); JRN-07 and JRN-04 ([journeys](../05-journeys.md));
BR-TRUTH-003, BR-REV-001..009, BR-AUD-001/004, BR-AMD-001..003, BR-TIME-003
([shared rules](../04-business-rules.md)); the catalog v5 row
`trip.trip.close_for_other O O O O - - - - -` ([permissions](../07-permissions.md));
BR-OFFL-006 and the outcome row "Trip already closed by someone else"
([offline sync](../10-offline-sync.md)); REQ-07.6 ([traceability](../17-traceability.md));
08-state-model §§1, 3, 4; the evidence of [TRIP-001](f4-trip001-trip-domain.md)
(honest meter usage, `NOT_MEASURED`), [TRIP-002](f4-trip002-trip-persistence.md)
(held `TRIP_ALREADY_CLOSED`), [TRIP-004](f4-trip004-trip-review.md) (review,
`TripStructure`, self-approval), [TRIP-015](f4-trip015-late-events-amendments.md)
(amendments, held events), [METER-004](f4-meter004-reading-correction.md),
[OFFL-009](f4-offl009-post-revocation-window.md) and [IDN-010](f1-idn010-offboarding.md);
EDR-178..181, EDR-202..206, EDR-220..223, EDR-234..237, EDR-250/251.

IDs used: B-BR-EMP-399..403, BUC-EMP-202..203, EDR-257..259 (all of them).
Migration `000235_organization_trip_closures.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-399..403 and BUC-EMP-202/203 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-014 — Missing final reading and supervisor closure (2026-11-15)";
decisions EDR-257 (server-only pure closure, separation, no reading), EDR-258
(persistence, replaced guard bodies, no invented end instant) and EDR-259
(routes, authority, error mapping, counts) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Design answers:

- **Who and how.** `trip.trip.close_for_other` (OWNER, ADMIN, FLEET_MANAGER,
  SUPERVISOR at ORG in catalog v5) covering the trip through the EMP-TRIP-004
  `TripStructure` (ORG, or UNIT/COST_CENTRE/TEAM/ASSET through the stored
  start snapshot); fresh company proof, ACTIVE organization, operational
  revision 1 as CAS. Online only (a panel action), so no signed operation and
  no Kotlin mirror (EDR-257, the EDR-250 precedent).
- **Missing evidence.** The closure carries a reason and a missing-reading
  reason (METER-001 text profile) and no reading, value, segment or distance:
  the request has no such field (unknown keys are `MALFORMED`), the table has
  no such column, no `TRIP_END` reading is created and the pure usage is
  `NOT_MEASURED`/`READING_ABSENT`. The review decision context shows the
  closure beside the absent finish. A reading taken later stays a separate
  reading; tying it to the trip is an amendment (retained).
- **TRIP-002 interplay.** The driver's later `trip.finish` is held
  `TRIP_ALREADY_CLOSED` with its declared reading kept in the held finish (the
  existing held-finish guard already admits any COMPLETED trip); every later
  logbook event is held `TRIP_ALREADY_CLOSED` because a closure has no device
  instant to compare (BR-OFFL-006, BR-TIME-003).
- **TRIP-004 interplay (BR-REV-003).** The closer is excluded like an author
  from every review decision of the trip (Go `DecideRevision` and the
  review-entry guard); closing one's own trip (author or driver, by account,
  membership or canonical employee) is `SELF_APPROVAL`. The closure never
  submits or approves: the author may still `trip.submit` it.
- **No invented end.** A closed trip holds its asset only until the closure;
  the trip-backed active-use query counts a trip without a finish only while
  `IN_PROGRESS`, so the next start on the vehicle is accepted.

## Implementation

- Domain (`trip/domain/closure.go`): `Trip.Close(scope, CloseInput)` (scope →
  trip → closer refs → absence → `ErrSelfClosure` → state), `Closure`,
  `Trip.Closed()`; `MeterUsage` and `Log` honour a closure (two guarded
  clauses in `trip.go`/`logbook.go`). The C1 command vocabulary,
  `Transition`, `trip-v1.psv`, `logbook-v1.psv` and Kotlin are unchanged.
- Application (`trip/application/closure.go`): `ClosePolicy`, `CloseCommand`
  (shape, reasons, SHA-256 digest), `ClosureRecord` (new `Record.Closure`),
  `CloseResult`, `OpenTrip(Page)`, `ClosureRepository`, `ClosureService`
  (live decision proved before and after the store, result shape re-proved),
  `CloseDomain`; `StoredDomain` rebuilds a closed trip; `DecideRevision`
  takes the closer as an extra excluded person (variadic, existing calls
  unchanged); `ReviewView.Closure`.
- Adapters: `trip/adapters/closure_store.go` (authority holding the root for
  writes → trip row lock → replay by operation proved by the audit event →
  revision/state CAS → separation → pure closure → insert + one
  `trip.trip.closed` audit event → authority again; open list keyset);
  `ReadTripTx` reads the closure of a completed trip without a finish; the
  finish handler holds a finish of a closed trip; the review store shows the
  closure and passes the closer to `DecideRevision`.
- Authority: `TripReviewAuthority.CheckTripCloseTx`; `TripStructure` resolves
  `trip.trip.close_for_other`; the company policy allow-list admits both
  closure policies.
- HTTP (`trip/adapters/http/closure.go`): `POST …/trips/open/list` and
  `POST …/trips/{trip_id}/close`; the review read projects `closure`.
- Runtime: `enterprisesuite` registers the routes, `ClosureAuditDefinitions`
  and the admission class `trip.close`.
- Migration 000235: forced-RLS, insert-only `org_trip_closures` (BEFORE guard
  under root and trip locks, AFTER completion, deferred publication with the
  audit event, deferred FK to the audit event); replaced bodies, every other
  line kept: `org_trip_completion_guard` (220), `org_trip_event_source`,
  `org_trip_held_event_source`, `org_trip_review_entry_source` (229). No
  usage source is redefined (no asset/unit/cost-centre key). sqlc: new
  `db/queries/trip/closures.sql`; `TripUsesOfAssetAt` counts a trip without a
  finish only while `IN_PROGRESS`; `sqlc.yaml` lists 000235.
- Contract and inventories: OpenAPI paths and schemas
  (`EnterpriseTripOpenListRequest/Item/Page`, `EnterpriseTripCloseRequest`,
  `EnterpriseTripClosure`, `EnterpriseTripCloseResult`) and the nullable
  `closure` of `EnterpriseTripReviewView`; route inventory
  `trip.trip.close_for_other` ROUTED (representative the open list);
  inventory counts 149 normal (29 routed / 55 not routed / 25 special);
  full-router list; matrix representative and private-state snapshot
  (290 cells, 116 / 174); AUDITOR walk 149 / 30 / 119.
- Test fixtures: `tripClosureGrant` (conditional on the 000235 table) is
  applied by `tripGrants`, `tripReviewGrants` and `eventGrants`, because the
  completion, logbook and review guards now read `org_trip_closures`.

Shared trip intake files touched (minimal, listed for the parallel TRIP-007/
011/012/023 agents): `trip/application/trips.go` (one `Record.Closure`
field), `trip/application/events.go` (one `StoredDomain` branch),
`trip/application/revisions.go` (`DecideRevision` variadic closer),
`trip/application/review.go` (`ReviewView.Closure`, one validation clause),
`trip/adapters/store.go` (`ReadTripTx` closure branch),
`trip/adapters/handlers.go` (one finish hold condition),
`trip/adapters/review_store.go` (closer exclusion, closure in the view),
`trip/adapters/http/review.go` (closure projection),
`trip/domain/trip.go`/`logbook.go` (one field, two clauses),
`db/queries/trip/trips.sql` (active-use query), test fixtures
`trip_intake/review/event_integration_test.go` (one grant call each).

## RED

Shape first (rules, EDRs, migration as written, exported names with stub
bodies returning `errNotImplemented`/`ErrUnavailable`, routes, OpenAPI and
inventories), then the frozen tests:

- Domain (`go test -race -json ./internal/modules/trip/domain -run 'TestTripClose|TestTripClosure'`),
  log `/data/dev/worktrees/f4-logs/trip014-go-red-domain.jsonl`: 3/3
  top-level failing.
- Application, log `trip014-go-red-application.jsonl`: 4 new top-level, 3
  failing; `TestClosePoliciesDrift` passed because the policy table is shape.
- PostGIS (`-race -tags=integration -p 2 -run '^TestTripClosure'`), log
  `trip014-go-red-integration.jsonl` (2 min 20 s): 6/6 top-level and 3/3
  subtests failing, all behavioural (`unavailable`/`not implemented` from
  the stubs; the race subtests' second request never queued because the
  stubbed closure did not reach the lock).
- The OpenAPI/apicontract test was written with the contract blocks and
  passed at once; no contract RED is claimed.

## GREEN and REFACTOR

- Unit: trip packages pass under race (domain, application, adapters, http).
- PostGIS focused, log `trip014-go-green-integration-1.jsonl` (52 s): 6/6
  top-level and 3/3 subtests pass on the first run; no expectation changed.
- REFACTOR: toolchain gofmt alignment only.

## Named negatives and acceptance → tests

| Requirement | Test(s) |
|---|---|
| AC: closes under approved authority; missing evidence visible; no distance invented (BUC-EMP-202, JRN-07) | `TestTripClosureClosesAnotherMembersOpenTripWithoutInventingEvidence` (open list; COMPLETED rev 2, closure facts, `NOT_MEASURED`/`READING_ABSENT`; no finish, no `TRIP_END` reading, reading count unchanged; exact closure row and one audit event without reason text; review read shows the closure and no finish); `TestTripClosureThroughComposedCompanyHTTP` |
| Self-decision where prohibited | ADMIN closing its own trip `ErrSelfClosure` / HTTP 409 `SELF_APPROVAL`; the closer approving or returning the closed trip `SELF_APPROVAL` while another reviewer approves; database: self-closure and closer review entry refused (`TestTripClosureSchemaIsForcedRLSInsertOnlyAndGuarded`); unit `TestDecideRevisionExcludesTheCloser`, `TestTripCloseFailureOrder` |
| Nonexistent meter value | HTTP `final_reading`, `value`, `distance` keys → 400 `MALFORMED`, nothing written; apicontract refuses them; no `TRIP_END` reading; after approval, an amendment "correcting" the never-recorded final reading is `ErrInvalidAmendment` and stores nothing |
| Foreign trip → NOT_FOUND | another organization's owner closing and listing; unknown trip; HTTP 404 on another organization's path and an unknown trip |
| Uncovered structural scope → FORBIDDEN | SUPERVISOR of another UNIT, FLEET_MANAGER of another ASSET; AUDITOR, FINANCE, APPROVER, DRIVER; a UNIT grant cannot list the organization's open trips; the trip's UNIT and ASSET grants close it |
| Stale second closure (CAS → CONFLICT) | revisions 1 and 2 after a closure → `ErrClosureConflict`; HTTP 409; `two-closures` race |
| Concurrent driver finish vs closure, both orders | `TestTripClosureRacesTheDriverFinishInBothOrders/{finish-first,closure-first,two-closures}` (both requests queued behind a held organization root lock, observed in `pg_stat_activity`): finish first → closure CONFLICT, nothing written; closure first → finish held `TRIP_ALREADY_CLOSED` |
| TRIP-002 hold kept for review | the driver's later finish held with its declared reading (`org_trip_held_finishes.reading_value`), replayed byte for byte; later STOP and NOTE events held `TRIP_ALREADY_CLOSED` |
| Replay / reuse | same operation and digest → first result `replayed=true`, no second row or audit; another digest → `OPERATION_ID_REUSED` (store and HTTP) |
| Asset freed, no invented end | the next start on the vehicle is accepted and is the only open trip |
| Forced RLS, insert-only, guards | `TestTripClosureSchemaIsForcedRLSInsertOnlyAndGuarded` (finished trip, self-closure, foreign closer employee, blank reason, unaudited closure at commit, bare completion 55000, UPDATE/DELETE/TRUNCATE 55000 with privileges, deleting the closed trip 55000, nothing visible without the organization) |
| Populated upgrade from the 000232 head | `TestTripClosurePopulatedUpgradeFailureRecoveryAndPreviousImage` (submitted and running trips; injected failure leaves schema/functions/data; recovery once; no-op replay; previous image compatible; forced-RLS migrator ownership; the pre-existing running trip then closed, its finish held, the pre-existing submitted trip decided under the replaced guard) |
| Contract | `TestTripClosureContractShapesAndGuards`; inventory and full-router tests in `cmd/api`; matrix and AUDITOR walk |

## Validation

Go 1.27.4 toolchain from `backend`, `GOCACHE=/data/dev/cache/f4-go-build`,
real PostGIS through `orgtest` child databases, a throwaway MinIO project
`f4-trip014-s3` from `infra/compose.validation.yml` (removed with `down -v`):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/trip/... ./internal/modules/organization/... ./internal/platform/{enterprisesuite,apicontract}/... ./cmd/api/...   # clean
sqlc vet && sqlc diff                                                 # clean
go test -json -race -tags=integration -count=1 -p 2 -timeout=90m ./internal/modules/organization/adapters ./internal/modules/{trip,membership,fleet,orgsync}/... ./internal/platform/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
python3 docs/enterprise/tools/check_plan.py                           # repo root
git diff --check
```

- Race/integration union, log `trip014-db-union.jsonl` (39 min 45 s;
  `organization/adapters` 2346 s, `apicontract` 1740 s under load): 37
  packages, 35 pass and 2 have no test files (`orgtest`, `legalterms`),
  1261 top-level / 9698 named tests, 0 failures, 0 skips. It includes every
  `TestTripClosure*` test, the authorization matrix at 290 cells
  (116 / 174), the AUDITOR walk at 149 / 30 / 119, every earlier trip,
  review, amendment, period, mission, event and late-intake suite (with the
  closure grants), `migrate` on the full chain and the storage-dependent
  document/export/auditor suites with the throwaway MinIO.
- Untagged whole module, log `trip014-untagged-all.log`: exit 0, 223 packages ok, 0 failures.
- Static gates, log `trip014-staticcheck.log`: staticcheck plain and
  `-tags=integration` clean; toolchain gofmt empty; vet (both tags) clean;
  `sqlc vet`/`sqlc diff` clean.
- OpenAPI (`vacuum lint -r contracts/openapi/vacuum-rules.yaml`): 0 errors;
  1071 warnings / 660 informs versus 1064 / 660 on the base head (the
  existing enterprise path-pattern and description-style rules), log
  `trip014-vacuum.log`.
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean.
- Secret/PII review of the staged diff: synthetic identifiers and the
  disposable MinIO/PostGIS credentials only in local commands, none in the
  diff; no real name, CNPJ, CPF, plate or location.
- No Kotlin, Android, shared vector, job or deployment change, so no Gradle
  run applies; no emulator or device was used.

## Retained obligations

- Binding a later reading (JRN-07 step 3) to a closed trip through a reviewed
  amendment that adds a missing reading (EMP-TRIP-015 retained item), and the
  indicator showing "closed by a supervisor" (METER-006/KPI).
- Resolution of the held finish and events of a closed trip (EMP-TRIP-007).
- Submission of a closed trip whose author can no longer submit (an ended
  member, EMP-TRIP-018 retained) and the author's own result read.
- Asset-overlap resolution of a later start declared inside a closed trip's
  span (EMP-TRIP-007).
- The panel screens (SCR-TRIP-03, EMP-TRIP-006/022), notifications, a
  TEAM-scope closure fixture, a configurable "left open" threshold
  (JRN-07 step 1 flags 30 hours).
- Production runtime grants (not provisioned here): company runtime
  `SELECT, INSERT ON org_trip_closures`; the signed intake principal and
  every reviewer `SELECT ON org_trip_closures` (completion, logbook and
  review guards read it); the migrator `REFERENCES ON org_trips,
  org_memberships, org_employee_identities, org_audit_events` and ownership of
  the four replaced functions.
- No C1, M4 or F4 acceptance, deployment or real data is claimed.

## Merge with EMP-TRIP-011 (pause, resume, cancel) — merged-tree validation

At the coordinator's request the branch merged `codex/enterprise-f4`
(`45c3baf3`: EMP-TRIP-011 with migration 000233, EMP-OFFL-005, EMP-GPS-001
and the PR #224 merge) in merge commit `cc5b5985`, after the two task
commits. Conflicts were resolved as unions (contract sections, EDR table by
number, sqlc schema list, `Record` fields, trip domain fields, logbook and
usage clauses, the finish hold, `ReadTripTx`, intake fixture grants). The
closure was rebased on the trip lifecycle, keeping every 000233 line:

- 000235 now starts from the 233 bodies of `org_trip_completion_guard`,
  `org_trip_event_source` and `org_trip_held_event_source` and redefines
  `org_trip_revision` (233 body) to count the closure like a finish; the
  review-entry source keeps its 229 body (000233 did not redefine it).
- A `PAUSED` trip is open: closable at its current operational revision
  (stale revision `CONFLICT`), completed at the next revision, listed with
  its state and revision; a `CANCELLED` trip is closed (`CONFLICT`, nothing
  written); a resume after the closure is held `TRIP_ALREADY_CLOSED`.
- `Trip.Close` admits `PAUSED` and moves the revision; `Record.Revision()`
  counts the closure; the active-use query keeps the lifecycle rule and
  counts a trip without a finish or cancellation only while open.
- New tests: domain `TestTripCloseTreatsPausedAsOpenAndCancelledAsClosed`,
  PostGIS `TestTripClosureTreatsPausedTripsAsOpenAndCancelledAsClosed`; the
  closure upgrade migrator also owns `org_trip_revision`.

Merged-tree gates: toolchain gofmt empty, vet (both tags), `sqlc vet`/`diff`,
staticcheck plain and integration clean (`trip014-merged-staticcheck.log`);
focused PostGIS `^(TestTrip|TestOffboarding)` 87/87 top-level pass
(`trip014-merged-focused.jsonl`, including every TRIP-011 lifecycle test);
MERGED_UNION_PLACEHOLDER
