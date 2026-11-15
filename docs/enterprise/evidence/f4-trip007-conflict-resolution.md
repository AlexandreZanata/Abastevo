# EMP-TRIP-007 — Asset-overlap conflict resolution

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task on `codex/enterprise-f4-trip007` (worktree
`/data/dev/worktrees/f4-trip007`) based on the F4 head `fe7ec66d` (main
`93f3a88d` with PR #223, EMP-TRIP-009/013 and the origin/dev Play Store
work), then merged with the F4 head `45c3baf3` (EMP-OFFL-005 and
EMP-TRIP-011) before its final gates. Real dependencies used: the OFFL-008 review items, resolutions,
own-result read and audit (migration 000199, `fleet/adapters/meter_review*`);
the EMP-TRIP-002 trips and held starts/finishes, EMP-TRIP-003 held events,
EMP-TRIP-015 held submissions (STALE_REVISION); the EMP-TRIP-004 trip review
company proof and policies; FLEET-005 allocation periods; METER-001/003
readings and classification; AUTH-006 structural scope; the company HTTP
entry. Nothing is faked; synthetic data only. No Android, panel, job or
deployment change.

Read: AGENTS, FAST_EXECUTION, 00-conventions; the EMP-TRIP-007 block and the
TRIP-002/003/004/005/015/023 blocks ([EP06](../backlog/ep06-trips.md));
[trips](../modules/trips.md) §§4–8 (BR-TRIP-002/003);
[10-offline-sync](../10-offline-sync.md) §§3–8 (BR-OFFL-004/006/007/009,
UC-OFFL-04, the review item lifecycle, SCR-OFFL-03); BR-REV-003/007
([04-business-rules](../04-business-rules.md)); VAL-07
([15-validation](../15-validation.md)); the `trip.trip.review` row of
[07-permissions](../07-permissions.md); EDR-117..121, 202..206, 220..223,
234..237 and the latest rows of [16-decisions](../16-decisions.md); the
[F4 ledger](f4-execution.md) and the evidence of EMP-TRIP-002, TRIP-003,
TRIP-004, TRIP-015, FLEET-007 and [OFFL-008](f2-offl008-real-review.md).

IDs used: B-BR-EMP-407..411, BUC-EMP-206..208, EDR-262..264 — all of them; no
pre-allocated ID is left unused. Migration
`000237_organization_trip_conflict_resolutions.sql` (needed: the 199
resolution guards knew only the meter owner and the 220 trip source refused
every held start's identity).

## Rules frozen before behaviour

Observed before any change: the four trip owners keep their held facts as
version-1 OFFL-008 review items (`org_trip_held_starts`, `_finishes`,
`_events`, `_submissions`); the meter queue, read and resolution join
`org_meter_held_readings`, so a trip item was invisible there and the 199
resolution source guard refused any decision on it; the signed own-result
read proved items through the meter held row, so a selector of a held trip
item made the whole answer `NOT_FOUND` (the Android poller polls every
retained REVIEW entry); `org_trip_source` refused every held start identity
(23505); `org_sync_review_resolutions` required a reading for an accepted
resolution.

Rules B-BR-EMP-407..411 and use cases BUC-EMP-206..208 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-007 — Asset-overlap conflict resolution (2026-11-15)"; decisions
EDR-262 (outcomes: refuse every held trip fact, accept only a held start as
its declared trip and only when no stored trip of the asset is in progress or
finished after its declared start; server-only rules, no Kotlin mirror),
EDR-263 (routes, `HeldTripStructure`, self-review including the declared
driver, owner-neutral own-result read, counts) and EDR-264 (migration 000237)
in [16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

## Shape

- Migration `000237_organization_trip_conflict_resolutions.sql` (EDR-264):
  no table and no foreign key. The `org_sync_review_resolutions` outcome
  check is found by its definition and replaced by
  `org_sync_review_resolutions_outcome` (`ACCEPTED` needs entity revision 1;
  a refusal states a reason and names no reading). Replaced, keeping every
  check of their latest bodies: the 199 resolution source guard (per-owner
  branches; meter unchanged but `IS DISTINCT FROM` its held reading; a held
  start's held reading or none; held finishes, events and submissions
  `REFUSED` only; the declared driver added to the self-review refusal), the
  199 resolution publication guard (audit event for every owner; the meter
  reading for meter items; the stored trip of the same operation at the
  original receipt and its reviewed `TRIP_START` reading for an accepted
  start; the readiness effect), the 216 observation guard (owner read first;
  a reviewed `TRIP_START` reading equal to the held start's reading under its
  accepted resolution, received at the original receipt), the 199 effect
  source (accepted receipt; else an accepted resolution of the operation
  must exist; then the reviewed meter or trip-start publication, each its
  own statement, so an ordinary or unresolved effect reads only what the
  199 body read), the 220 trip source (a held start's
  identity admitted only for its own accepted resolution with every held
  fact equal; received at the original receipt; refused `23514` when any
  stored trip of the asset is in progress or finished after the declared
  start) and the 220 trip publication (accepted receipt, else the new
  `org_trip_review_published`). Usage sources are unchanged.
- Application `trip/application/conflicts.go`: the state and decision
  vocabularies, `ConflictPolicy` (= the EMP-TRIP-004 `trip.trip.review`
  policies), `HeldCode`, the views (`ConflictView`, `ConflictFact`,
  `HeldStartFact`, `HeldEventFact`, `ConflictTrip` with the `Overlaps` rule),
  `ConflictFilter`, `ConflictDecision` (reason profile, digest tag
  `trip.conflict.resolve`), the pure `DecideConflict` and `SelfReview`
  rules, `HeldStartTrip` (the EMP-TRIP-001 trip of an accepted held start)
  and `ConflictService` (live decision before and after the store, full
  result validation). Server-only: no Kotlin mirror and no PSV (EDR-262).
- Store `trip/adapters/conflicts.go` (+ sqlc `db/queries/trip/conflicts.sql`,
  a new file so the shared trip query files are untouched): authority →
  item lock → replay by operation → view → revision → self-review →
  acceptance facts under the asset row lock → one audit event with the
  resolution and, for an acceptance, the reviewed reading, the trip and the
  readiness effect → authority. `ConflictAuditDefinitions` is the OFFL-008
  `offl.review.resolved` event.
- Fleet port `ReviewedReadingRepository.ApplyReviewedEmbeddedTx` (new files
  `fleet/application/meter_review_embedded.go`,
  `fleet/adapters/meter_review_embedded.go`) implemented by
  `MeterReviewStore`: meter lock, METER-003 classification at the original
  receipt, insert with `review_resolution_id`. The OFFL-008 own-result read
  (`OwnMeterReviewResults`) now proves an item through the owner-neutral
  `ReadOwnReviewProof` (new `db/queries/fleet/review_owners.sql`) instead of
  the meter held row.
- Authority: AUTH-006 kind `HeldTripStructure` in `resource_structure.go`;
  `TripConflictAuthority` (new file) reuses the trip review company proof
  (`trip_review_authority.go` `check` now takes the expected kind).
- HTTP `trip/adapters/http/conflicts.go`: three routes (EDR-263), strict
  bodies, the EMP-TRIP-004 refusal mapping; composed in
  `enterprisesuite/routes.go` with the meter review store as the reading
  port.
- Contract: OpenAPI 3 paths and 4 schemas (`EnterpriseTripConflictView`,
  `…Page`, `…Fact`, `…Trip`; the list and resolve bodies reuse the meter
  review request and result schemas); inventory `trip.trip.review` 6 routes
  (normal 147 → 150, routed 28, not routed 56, specials 25); router test
  paths +3; auditor walk 150 / 30 / 120; matrix unchanged (no new
  permission: 280 cells, 112 / 168); new `apicontract`
  `TestTripConflictContractShapesAndGuards`; `TestEnterpriseSyncComponents`
  lists the three paths. README: runtime and migrator grants.

## RED

Logs in `/data/dev/worktrees/f4-logs/trip007-*`. The application rules and
the store were drafted before their tests (an honest deviation from
test-first order, as in TRIP-002); RED was obtained by freezing the tests,
the migration and the contract and running them against stub bodies.

- Application (`DecideConflict`, `SelfReview`, `Overlaps`, `HeldCode`,
  `NewConflictDecision`, `Digest`, view and filter validation stubbed to zero
  values) → `trip007-app-red.jsonl`: 5/5 new top-level tests and 5/8
  subtests failed; the three passing subtests are the admissible
  acceptances the stub happens to admit (no trip, finished before, finished
  exactly at the declared start).
- Router/contract with the routes wired and the OpenAPI and inventory at
  `HEAD` → `trip007-cmd-red.jsonl` (4/22 top-level failed: inventory
  coverage, meter descriptor inventory, company composition, full router)
  and `trip007-contract-red.jsonl` (34/92 failed: every company-route gate
  sees an undocumented organization route, the sync component list and the
  new conflict contract test).
- PostGIS behaviour (migration present, the three store entry points
  stubbed, the own-result proof back on the meter held row) →
  `trip007-db-red-behaviour.jsonl`: 8/8 top-level and 6/6 subtests failed.
  Two failed for the wrong reason and were fixed before GREEN, no
  expectation changed: the populated-upgrade fixture's meter held reviews
  (`meter: invalid evidence` on the trip fixture's meter; the meter
  resolutions were dropped from that fixture, see Retained) and the schema
  test's "accepted resolution without its trip" case, which hit the
  deferred audit foreign key (23503) before the publication guard (the case
  now records its audit event first). Their reruns against the same stubs
  (`trip007-db-red-behaviour2.jsonl`, `trip007-db-red-upgrade.jsonl`) fail
  only at the stubbed store call after every database assertion passed.
  The first behaviour run was stopped once (my own process) because the
  race fixture created a reviewer while the test held the root lock; the
  reviewer is now created before the barrier.

## GREEN and REFACTOR

- Application: `go test -race ./internal/modules/trip/application/`
  (`trip007-app-green.jsonl`): the five conflict tests and their 8 subtests
  pass; trip and fleet unit packages pass under `-race`.
- PostGIS before the merge (`-race -tags=integration`, real ordinary
  NOSUPERUSER/NOBYPASSRLS runtime role through `orgtest`) →
  `trip007-db-green.jsonl`: 8/8 top-level
  and 6/6 subtests pass. The first GREEN run failed every test with
  `unavailable`: the held fact reader inverted its "exactly one of reading
  and stated absence" check (`(reading == "") == reason`), a store defect
  caught by the tests; fixed to `(reading != "") == reason`, no expectation
  changed.
- Router and contract: `cmd/api` 22/22 top-level, `apicontract` 92/92
  top-level (`trip007-cmd-green.jsonl`, `trip007-contract-green.jsonl`).
- Added after GREEN as characterization (not counted as RED):
  `trip/application` `TestHeldStartTripRebuildsTheDeclaredTrip` (the
  EMP-TRIP-001 trip and start day of an accepted held start, with a reading
  or a stated absence; a finish or a reading without its value refused) and
  `trip/adapters/http` `TestConflictRequestsRecordsAndProjection` (strict
  bodies, refusal mapping, route rules, one projection per held finish,
  event and submission, an incomplete view refused). Coverage: trip
  application 61.1%, trip HTTP 21.6% (the store and routes are covered by
  the PostGIS tests).

## Merge with EMP-TRIP-011 (second commit)

After the first commit `e81d6364` the coordinator integrated EMP-TRIP-011
(pause, resume and cancellation; migration 000233 redefining nine trip and
review functions, `PAUSED`/`CANCELLED` trips, insert-only
`org_trip_transitions` and held `org_trip_held_transitions` with
`TRIP_ALREADY_CLOSED`/`STALE_REVISION`) on `codex/enterprise-f4`
(`45c3baf3`). The branch merged it normally (only the append-only contract,
EDR table and README sections conflicted; resolved by union, theirs first)
and the second commit aligns TRIP-007 with it:

1. 000237 redefines the trip publication from its 000233 body (every line
   kept: the finish published at `org_trip_revision`), adding only the
   reviewed start publication; 000237 redefines no other 000233 function.
2. The trip source's overlap re-proof and the conflict queries treat a
   `PAUSED` trip as holding the asset and a `CANCELLED` trip as ending at its
   cancellation (the 000233 held-source rule), and a conflicting trip's
   `finished_at` is its finish or its cancellation.
3. The held lifecycle commands are queue owners: listed and read with their
   command, named revision and reason (`fact.transition`), structure through
   the trip they name, refusal only (the resolution source admits
   `REFUSED` for them; acceptance is `CONFLICT`).
4. OpenAPI (`operation_type` +3, `fact.transition`), the application view,
   the store, the HTTP projection, `HeldTripStructure` and the README grants
   follow; new tests: a held pause in the queue test (listed, read, refused,
   acceptance `CONFLICT`, the trip unchanged),
   `TestTripConflictCancelledTripEndsAtItsCancellation` (a paused trip blocks
   an acceptance; after the cancellation a start inside the cancelled trip is
   still `CONFLICT` and one after it is accepted), and pure cases for
   cancelled and paused trips and lifecycle views.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: concurrent start evidence stays readable | `TestTripConflictQueueListsAndReadsEveryHeldTripFact` (VAL-07 held start with its exact reading, run clock, declaration, author and the conflicting trip; a held finish, event and submission with their facts; keyset pages, state filter, reads equal to the list) |
| AC: resolution preserves one active trip per asset | `TestTripConflictResolutionNeverMakesTwoTripsActive/{accept-then-start,start-then-accept,accept-first-of-two,accept-second-of-two}` (barrier on the organization root; both orders of an acceptance and a new start, and of two acceptances; exactly one trip in progress; the loser `CONFLICT` or held `ASSET_BUSY`); `TestTripConflictRefusalKeepsTheEvidenceAndAnswersTheAuthor` (an overlapping start is never accepted, before and after the winner finishes); pure `TestConflictAcceptancePreservesOneActiveTripPerAsset` |
| Accept as the declared trip | `TestTripConflictAcceptanceStoresTheDeclaredTripAndItsPendingFinish` (trip equal to the held fact, received at the original receipt, unit snapshot and start day; `TRIP_START` reading under the resolution at the original receipt; readiness effect; the pending finish `DEPENDENCY_PENDING` before and `ACCEPTED` after; original ACK replayed byte for byte; acceptance replay) |
| Self-review | `TestTripConflictRefusesSelfReviewUncoveredScopesAndAuditors` (ADMIN author holding `trip.trip.review`: `SELF_APPROVAL`, nothing written); HTTP `SELF_APPROVAL`; pure `TestConflictSelfReviewCoversAuthorAndDriver` (account, membership, merged employee, declared driver); database `P9006` branch kept from 199 |
| Second active trip produced by a resolution | race test above; database re-proof `TestTripConflictSchemaGuardsReproveEveryAcceptance` (overlapping accepted trip `23514` even with an accepted resolution) and the partial unique index |
| Repeated resolution (replay/CAS) | refusal test: two reviewers racing in both orders → one resolution and one audit event, the other `CONFLICT`; replay returns the stored bytes (`replayed`); same operation with another reason `OPERATION_ID_REUSED`; a second decision and a stale revision `CONFLICT`; HTTP the same codes |
| Loss of the conflict evidence | refusal test: review item, held row and original ACK bytes unchanged after the decision; UPDATE/DELETE of the held start, the resolution and the item refused to the runtime role (42501 or 55000 accepted; the role holds no UPDATE/DELETE and the tables keep their 199/220 immutability triggers); acceptance test: held evidence unchanged after acceptance |
| Foreign organization, unknown and meter items | queue test: another organization's owner gets `NOT_FOUND` for read and resolve and an empty queue; an unknown item `NOT_FOUND`; HTTP foreign path and unknown item `NOT_FOUND`. A meter item on these routes (and a trip item on the meter routes) is excluded by the owner-type filters of the queries and the structure lookup; no test seeds a meter item in the trip fixture |
| Uncovered structural scope, AUDITOR | scope test: AUDITOR, FINANCE, DRIVER, a SUPERVISOR of another unit and a FLEET_MANAGER of another asset `FORBIDDEN` on read and resolve (held start and held finish); a UNIT grant cannot list the organization queue; covering UNIT and ASSET grants read, refuse and accept; HTTP AUDITOR `FORBIDDEN` on the three routes; AUTH-011 walk |
| Accepting a finish, event, submission or lifecycle command | scope test (`CONFLICT` for finish and event); queue test (held pause: acceptance `CONFLICT`, refusal recorded, trip unchanged); pure rule for all six owners; database `23514` for an accepted held finish |
| Paused and cancelled trips (EMP-TRIP-011) | `TestTripConflictCancelledTripEndsAtItsCancellation` (paused trip blocks an acceptance; a start inside the cancelled trip `CONFLICT`, one after its cancellation accepted, exactly one trip in progress); pure cases for cancelled and paused trips |
| Author's own result | refusal and acceptance tests: `OwnMeterReviewResults` returns the exact resolution to the author's source and refuses another member |
| Database re-proof | schema test: accepted finish (23514), accepted resolution without trip/reading/readiness (23514 at commit), effect without resolution (23514), held trip identity without its resolution (23505), overlap (23514) |
| Populated upgrade 000232 → 000237 | `TestTripConflictPopulatedUpgradeFailureRecoveryAndPreviousImage` (completed trip, held start, held finish; injected failure leaves functions, constraints and data unchanged; ordinary migrator owning the table and six functions; recovery once; no-op replay; previous image compatible; usage answers unchanged; the new outcome check present; receipts replay; refusal and acceptance on the upgraded schema) |
| Wire contract | `TestTripConflictContractShapesAndGuards`, `TestEnterpriseSyncComponents`, router inventory and full-router tests |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain,
`GOCACHE=/data/dev/cache/f4-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`,
`-p 2`; shared PostGIS control `127.0.0.1:32775` used only through `orgtest`
isolated child databases; a throwaway MinIO project `f4-trip007-s3`
(`infra/compose.validation.yml`, `storage` plus the `init` profile; removed
with `down -v` afterwards) for the S3-backed suites.

```sh
gofmt -l ./internal ./cmd ./db                                    # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...  # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/{trip,fleet,organization}/... ./internal/platform/{enterprisesuite,apicontract}/... ./db/queries/{trip,fleet}/... ./cmd/api/...   # clean
sqlc vet && sqlc generate && sqlc diff                            # clean (new review_owners.sql and conflicts.sql only)
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml   # 0 errors (warnings below)
python3 docs/enterprise/tools/check_plan.py                       # 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --check                                                  # clean
go test -json -race -tags=integration -timeout=90m -p 2 -count=1 ./internal/modules/organization/adapters ./internal/modules/trip/... ./internal/modules/fleet/... ./internal/modules/orgsync/... ./internal/modules/review/... ./internal/platform/... ./cmd/api/...
go test -count=1 -timeout=45m ./...
```

First union, before the merge (`trip007-db-union.jsonl`, 45 min, organization adapters
2,724 s while other agents' suites shared the machine): 34 packages passed,
2 without tests, 1,217 top-level / 8,504 subtests passed and 4 failed.

- `TestSyncIntakeDependencyClaimsReviewAndMissingProducer` (a real
  regression of the first 000237 draft): the generic sync fixture grants the
  effect guard only the columns the 199 body reads, and the redefined effect
  source read the review item's `operation_type` first, so an effect of an
  unresolved held operation failed `42501` instead of the guard's `23514`.
  The effect source now first requires an accepted resolution of the
  operation (the 199 columns) and only then reads the meter or trip
  publication; the resolution source reads the held finish, event and
  submission tables only for those owners. The migration is not applied
  anywhere, so it was corrected in place; no expectation changed. Focused
  rerun (`trip007-db-focused-fix.jsonl`, `TestSyncIntake*`,
  `TestTripConflict*`, `TestMeterReview*`, `TestOwnReviewResults*`): 44/44
  top-level and 34/34 subtests pass.
- `cmd/api` `TestPublicProcessIdentityAndSignedWrites`,
  `…DevelopmentPhotoPreview`, `…CommunityActions`: the public worker's
  evidence sweep missed the test's 20-second health deadline under the
  machine load; untouched public code, they pass alone
  (`trip007-cmd-public-rerun.jsonl`, 3/3 top-level, 2/2 subtests).

Final gates on the merged tree (after the EMP-TRIP-011 merge and the
second commit's alignment):

- `TestTripConflict*` on PostGIS (`trip007-db-green-merged.jsonl`): 9/9
  top-level and 6/6 subtests pass (the new cancellation test included).
- Race/integration union (`trip007-db-union3.jsonl`, 35.6 min,
  organization adapters 2,135 s): 38 packages, 36 passed and 2 without
  tests; 1,249 top-level and 8,614 subtests passed; 0 failures, 0 skips
  (the matrix 280 cells 112 / 168, the AUTH-011 walk 150 / 30 / 120 and the
  three public-process tests included).
- Untagged whole module (`trip007-untagged-merged.log`): exit 0, 224
  packages ok, 81 without tests.
- Static (`trip007-static-merged.log`): toolchain gofmt empty, build and
  vet (both tags) clean, staticcheck plain and integration clean, `sqlc vet`
  and `sqlc diff` clean; vacuum 0 errors, 1,074 warnings / 665 informs vs
  1,067 / 665 on the F4 head `45c3baf3` (the existing route patterns of the
  three routes); `check_plan.py` 0 problems; `git diff --check` clean.
- A pre-merge union run on the corrected migration was stopped (my own
  process) when the coordinator asked for the merge; its partial results
  are not counted.

Pre-merge untagged whole module (`trip007-untagged-all.log`): exit 0, 223 packages
ok, 81 without tests (run before the effect-source correction, which
changes SQL only); after it, the untagged packages that read the embedded
migrations (`platform/migrate`, `db/...`, quality, privacy and account
adapters) pass again.

The staged diff was reviewed for secrets and personal data: synthetic
identifiers, names and texts only; the disposable MinIO credentials appear
only in the uncommitted log environment file.

## Retained

- **EMP-TRIP-023 and the mission link** are not part of this task (not
  started in this worktree). Held starts of a later mission-linked start
  form will need their owner type in the queue.
- **Accept with another asset** (VAL-07, UC-OFFL-04 "accept with a change"):
  not offered (EDR-262); the held reading belongs to the declared asset's
  meter. Its owner is a later amendment-style decision.
- **Attaching a losing fact to the winning trip's timeline**: not done; the
  held fact stays readable through the conflict read. The panel timeline is
  EMP-TRIP-006.
- **Android presentation** of a trip resolution (the trip book's refused or
  accepted start): the Android poller already receives the resolution through
  the unchanged OFFL-008 wire; the trip-book projection is EMP-TRIP-005/021
  (no Kotlin change here).
- **EMP-TRIP-011 integration** is done in the second commit (paused trips
  hold the asset, a cancelled trip ends at its cancellation, held lifecycle
  commands are refusal-only queue items). A resolution of a held lifecycle
  command never applies the command.
- The queue reads operation schema 1 owners only (the only protocol schema).
- **Notifications** (the author on a resolution, reviewers on new items) and
  the 60-day expiry escalation: OFFL-008/NOTIF retained obligations.
- **Existing meter resolutions across the upgrade**: the populated upgrade
  image holds no prior `org_sync_review_resolutions` row (the trip fixture's
  meter could not produce a held meter review); the widened check is
  validated by `ADD CONSTRAINT` on the upgraded table and the meter review
  suites in the union exercise the replaced guards on 000237.
- **Production grants** (README subsection) and the panel screens
  (SCR-OFFL-03 for trips) remain to be provisioned and built.
- No C1, M4, F4 or gate acceptance is claimed.
