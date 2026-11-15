# EMP-TRIP-004 — Trip submission and administrative review

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2/C1 task (module TRIP, P0, type API, size M) in worktree branch
`codex/enterprise-f4-trip004`, opened from the F4 integration head
`0fb49112` (TRIP-001/002/017 integrated; migrations through 000224). It lets
the trip's author submit a COMPLETED trip through the signed intake and an
independent, scoped reviewer approve, return or reject the current review
revision with reason and audit. Operational completion alone gives no
administrative approval. Synthetic data only; no Android, panel screen, job,
deployment or wage/time output.

Sources read: the EMP-TRIP-004 block and the TRIP-001/002/003/005/006/007/015/
016 blocks ([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md)
§§3–8 (UC-TRIP-02); [08-state-model](../08-state-model.md) §1 (the C1 table:
DRAFT on the device, SUBMITTED on server receipt, RETURNED, APPROVED), §3
(trip review = BR-REV states plus VOIDED) and §4 rule 7 (loser gets
CONFLICT); BR-REV-001..009, BR-AUD-001/002/004, BR-TIME-003, BR-AMD-006
([04-business-rules](../04-business-rules.md)); BR-AUTH-003/004/005, the
role table, the catalog v5 row `trip.trip.review O O O O - O - - -` and
examples 1–2 ([07-permissions](../07-permissions.md)); BR-OFFL-002/004/006/
007/009, the ACK table and the offline matrix rows "Finish trip", "Correct a
returned record" and "Approve, return, reject"
([10-offline-sync](../10-offline-sync.md)); 12-ux-app §4 ("Encerrar e
enviar"); VAL-01 step 6 ([15-validation](../15-validation.md)); REQ-07.2; the
TRIP-001 evidence (review is a closed input, EDR-181), the TRIP-002 evidence
(a stored trip is DRAFT with no entry, EDR-205), APPR-001
([evidence](f3-appr001-review-lifecycle.md), `review-v1.psv`), FUEL-002
(first review entry rows), APPR-006 (period gate; trips' lock is
EMP-TRIP-016), METER-004 and the OFFL-008 held-review routes (company
decision precedent), EDR-160/161 (AUTH-006 structural sources).

IDs used: B-BR-EMP-342..347, BUC-EMP-172..174, EDR-220..223 (all of them).
Migration `000226_organization_trip_reviews.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-342..347 and BUC-EMP-172..174 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-004 — Trip submission and administrative review (2026-11-15)";
decisions EDR-220 (signed `trip.submit` on entity `trip_review` with the
review revision as expected revision; ACK mapping), EDR-221 (append-only
`org_trip_review_entries` and its database guards), EDR-222 (company routes,
OPEN composed with the decision, the `TripStructure` resource kind, audit
actions, error mapping) and EDR-223 (server-only binding to the APPR-001
lifecycle, no new shared oracle, what RETURNED means now) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Design answers the task asked to verify:

- (a) **Who submits and how.** The trip's author, offline-capable, through a
  signed `trip.submit` operation on the existing intake (EDR-220): the C1
  flow marks the finished trip "ready to send" on the device (12-ux-app §4,
  VAL-01 step 4) and 08-state-model §1 makes it SUBMITTED only on server
  receipt (BR-REV-002), so the Android producer (EMP-TRIP-005) queues it
  behind its finish (`depends_on`). A member-session route would be online
  only. A separate entity type `trip_review` keeps the review revision
  (expected revision and ACK entity revision 1) apart from the operational
  start/finish ACK revisions 1/2.
- (b) **Which operational states may be submitted.** Only `COMPLETED`
  (TRIP-001's retained note). An `IN_PROGRESS` trip is `RETRY`
  `DEPENDENCY_PENDING` (its finish is the dependency), never refused.
- (c) **Review routes.** `trip.trip.review` (OWNER, ADMIN, FLEET_MANAGER,
  SUPERVISOR, APPROVER at ORG in catalog v5; DRIVER, FINANCE, AUDITOR
  denied). Decision: fresh proof, configuration write, expected revision,
  admission class `trip.review`. Reads kept minimal: the per-trip decision
  context (needed to know the revision) and an organization-level queue
  (needed by the AUTH-011 matrix as an organization-level representative
  and by a reviewer to find submitted trips; only ORG grants cover it).
  Lists filtered by structural scope, the timeline and SCR-TRIP-02 remain
  EMP-TRIP-006.
- **What "return" allows now.** RETURNED records the reason and is not
  terminal; the correction loop (a new revision through events/amendments
  and RESUBMIT) is EMP-TRIP-015, so a `trip.submit` on a returned case stays
  `RETRY`/`UNAVAILABLE` and stores nothing (EDR-220). No cancel command.

## Implementation

- Migration `000226_organization_trip_reviews.sql`: forced-RLS, append-only
  `org_trip_review_entries` (org, trip, sequence, kind
  SUBMIT/OPEN/APPROVE/RETURN/REJECT, revision, resulting state, exact
  reason, actor membership and employee record, operation, receipt) with
  same-organization composite foreign keys to the trip, the membership and
  the employee record only — no asset, unit or cost-centre key, so no usage
  source is redefined; kind/state and reason-policy CHECKs; one SUBMIT per
  trip and one OPEN and one decision per revision (partial unique indexes),
  `UNIQUE(org, operation, kind)`. The BEFORE trigger takes the organization
  root and trip row locks and re-proves a COMPLETED trip, the APPR-001
  transition from the latest entry, the author of a SUBMIT (a `trip.submit`
  operation on entity `trip_review` by the trip's author membership with an
  active enrolled device) and, for OPEN and decisions, an active unmerged
  reviewer whose canonical employee (merge chain), membership and account
  differ from the trip's author and driver. A deferred constraint trigger
  publishes a SUBMIT only with its ACCEPTED ACK and effect at the entry
  revision, an OPEN only beside the decision of the same operation, and a
  decision only with its own `trip.trip.*` audit event by the reviewer's
  account. UPDATE/DELETE/TRUNCATE are refused (`org_trip_keep_history`).
- Application (`trip/application/review.go`): `trip.submit` codec
  (`ParseSubmit`), `ReviewCase` (stored entries → `review.Replay`),
  `Standing`, `SubmitGate`, `Submit`/`Decide` on `review.Case.Apply` with
  server facts (author, beneficiary = driver), `DecisionCommand` (reason
  profile applied through the APPR-001 lifecycle itself, SHA-256 digest),
  `ReviewService` (Decide/Read/List proving the live request decision before
  and after the store), policies and ports (`SubmitStore`,
  `ReviewRepository`).
- Adapters: `trip/adapters/submit.go` (the `trip.submit` owner reusing the
  trip intake helpers and `TripAuthority` unchanged: current
  `trip.trip.start` over the stored own trip, EDR-118 classification,
  trip-row lock, gate, SUBMIT insert, final re-classification; replay
  re-proves the stored entry); `trip/adapters/review_store.go` (submit store
  methods on `*Store` in a new file, `ReviewStore` for the company path:
  authority, trip-row lock, replay by operation proved by the audit event,
  APPR-001 over the stored history, canonical identities, entries and one
  audit event in `RecordChangeTx`, authority again); sqlc
  `db/queries/trip/reviews.sql`.
- Authority: `organization/adapters/trip_review_authority.go`
  (`TripReviewAuthority`: real company proof, ACTIVE organization for
  decisions, reviewer membership/employee); `resource_structure.go` gains the
  `TripStructure` kind (asset, stored unit and cost centre, the author's and
  driver's team links at the start receipt) for `trip.trip.review`;
  `company_authority.go` admits the two review policies.
- HTTP (`trip/adapters/http/review.go`): `POST /v1/orgs/{org_id}/trips/review/list`,
  `…/trips/{trip_id}/review/read`, `…/trips/{trip_id}/review/decide`
  (strict lossless bodies, keyset cursor bound to the caller).
- Runtime: `enterprisesuite` registers the `trip.submit` owner
  (`NO_ASYNC_FOLLOWUP` for ACCEPTED/REJECTED), the routes, the audit
  definitions and the quota class.
- Contract and inventories: OpenAPI owner `trip.submit`,
  `EnterpriseTripSubmitPayload`, three paths, tag `enterprise-trips` and the
  review schemas; route inventory routes `trip.trip.review` (representative
  the queue); inventory counts, full-router mounted list, authorization
  matrix representative and private-state snapshot, AUDITOR walk counts;
  `scripts/check-manifest.txt` lists `trip/adapters/http`.

Shared trip intake files touched (another agent changes the intake in
`f4-trip003`): none of `trip/adapters/handlers.go`, `intake.go`, `store.go`,
`trip/application/trips.go`, `codec.go` or `db/queries/trip/trips.sql`.
New files only in those packages (`submit.go`, `review_store.go`,
`review.go`, `reviews.sql`); `sqlc.yaml` gains the 000226 schema line; the
new upgrade test builds the TRIP-002 fixture on an older image through the
existing `tripState.handlers` method.

## RED

Shape first: rules and EDRs, the migration as written, exported names,
routes, OpenAPI and inventories, then the tests. The application and adapter
bodies were drafted before the tests (an honest deviation from strict
test-first order, as recorded by EMP-TRIP-002); RED was therefore obtained
with the frozen tests and migration against stubbed bodies: a temporary
`redStub` switch returned `unavailable` at the top of `ParseSubmit`,
`ReviewCase`, `SubmitGate`, `NewDecisionCommand`, `Submit`, `Decide`, the
three `ReviewService` methods, the submission owner's `Admit`/`Apply`/
`Confirm`, the four store methods, the three `ReviewStore` methods and
`TripReviewAuthority.CheckTripReviewTx` (script
`/data/dev/worktrees/f4-logs/trip004-scripts/red_stubs.py`, applied and
reverted; no stub remains).

- Unit (`go test -json -race -count=1 ./internal/modules/trip/...`), log
  `/data/dev/worktrees/f4-logs/trip004-go-red-unit.jsonl`: of the 10 new
  top-level tests 5 fail (submission codec ×2, review-case binding,
  lifecycle decisions, decision command) and 5 pass because they exercise
  unstubbed shape code only (owner constructor and descriptor, the stored-
  entry proof, store failure mapping, policies/queue filter, HTTP strict
  decoding and refusal mapping); the 23 pre-existing trip tests pass.
- PostGIS (`go test -json -race -tags=integration -count=1 -p 2
  ./internal/modules/organization/adapters -run
  '^(TestTripSubmit|TestTripReview|TestAuthorizationMatrixActualCompanyHTTP)'`,
  4 min 34 s), log `/data/dev/worktrees/f4-logs/trip004-go-red-integration.jsonl`:
  8/8 top-level fail, 27 subtests fail, all behavioural (`orgsync:
  unavailable` from the stubbed owner, 503 on the 20 allowed and variant
  `trip.trip.review` matrix cells); 0 skips. The populated upgrade reaches
  its post-upgrade submission and fails there (the migration is shape).
- The OpenAPI/apicontract test was written with the contract blocks and
  passed at once; no contract RED is claimed.

One fixture defect was found by inspection between RED and GREEN and fixed
without changing any expectation: the "two dependencies" MALFORMED case named
two unknown operations, which the framework answers `DEPENDENCY_PENDING`
before any owner; it now names the trip's accepted start and finish.

## GREEN and REFACTOR

- Unit, log `trip004-go-green-unit.jsonl`: trip packages 33 top-level /
  310 named, 0 failures, race.
- PostGIS focused, log `trip004-go-green-integration-1.jsonl` (6 min 2 s):
  the submission, decision, race, HTTP, matrix (230 cells, 90 allowed /
  140 denied) and AUDITOR walk (127 routes, 29 positive, 98 FORBIDDEN) tests
  pass; three failed for fixture reasons and were corrected without changing
  an expectation: the upgrade test processed its pre-upgrade trip before the
  intake store was set (now the TRIP-002 store), the cost-centre and
  COST_CENTRE-grant fixtures need `SELECT/INSERT` the runtime test role lacks
  (inserted as the administrator under the organization setting; SELECT
  granted for the EDR-160 role guard), and the append-only check needs table
  privileges to reach the 55000 guard (granted as FUEL-002 does). Reruns
  `trip004-go-green-integration-2.jsonl` and `-3.jsonl`: all pass.
- REFACTOR: staticcheck SA4006 (an unused route copy in the contract test)
  fixed; the contract test also checks the queue's guard metadata.

## Named negatives and acceptance → tests

| Requirement | Test(s) |
|---|---|
| AC: the reviewer decides one matching revision and records reason and audit | `TestTripReviewDecidesOneRevisionWithAuditReplayAndRefusals` (OPEN+APPROVE entries by the reviewer, one `trip.trip.approved` audit event with actor, target, digest and no reason text; RETURN keeps "Informe o motivo da parada"; REJECT); `TestTripReviewThroughComposedCompanyHTTP` |
| AC: operational completion alone gives no approval | a completed, unsubmitted trip is never decided (`ErrInvalidTransition`) and is not in the queue; submission and decisions leave the `org_trips`/`org_trip_finishes` rows byte-identical; unit `TestReviewCaseBindsTheStoredHistoryToTheTripStanding` |
| Submission offline through the signed intake (BUC-EMP-172) | `TestTripSubmitAcceptsOnlyTheAuthorsCompletedDraftAndReplays` (DEPENDENCY_PENDING while in progress and while its finish is pending; ACCEPTED rev 1 with one entry/effect/receipt/audit; byte-identical replays; `OPERATION_ID_REUSED`; second and stale submissions RETRY with nothing stored; MALFORMED shapes; another member FORBIDDEN) |
| Self-approval (BR-REV-003) | `TestTripReviewRefusesSelfApprovalUncoveredScopesAndAuditors` (the ADMIN author approving or rejecting its own trip); HTTP 409 `SELF_APPROVAL`; database guard in `TestTripReviewSchemaIsForcedRLSAppendOnlyAndGuarded`; unit account/membership/employee/driver-merge-chain cases |
| Stale revision (CAS; loser CONFLICT, nothing changes) | decision test (revision 2 → CONFLICT, history and audits unchanged); `TestTripReviewConcurrentDecisionsLeaveOneDecision/{approve-first,reject-first}` (both queued behind a held root lock; exactly one decision and one audit, the other CONFLICT); HTTP 409 |
| Missing reason | HTTP 422 for empty and ideographic-space reasons; unit command cases (blank, control, 281 scalars); database reason CHECK |
| Repeated / second decision | replay returns the identical result with `replayed=true` and no second entry or audit; another digest → `OPERATION_ID_REUSED`; a second decision on the decided revision → CONFLICT (store and HTTP) |
| Another organization's trip → not found | decision test (owner of a second organization naming the trip: decide and read NOT_FOUND; unknown trip NOT_FOUND); HTTP 404 on another organization's path and an unknown trip |
| Uncovering structural scope → FORBIDDEN (AUTH-006) | `TestTripReviewRefusesSelfApprovalUncoveredScopesAndAuditors`: SUPERVISOR of another UNIT, APPROVER of a COST_CENTRE the trip does not carry, FLEET_MANAGER of another ASSET refused (decide and read); the trip's UNIT and its ASSET covered; a UNIT grant cannot list the organization queue |
| AUDITOR → FORBIDDEN | same test (and FINANCE, DRIVER); HTTP 403 with `trip.trip.review`; AUDITOR walk (3 new routes FORBIDDEN, private state unchanged) |
| Authorization matrix | `TestAuthorizationMatrixActualCompanyHTTP` (new `trip.trip.review` representative: 5 allowed roles, 5 denied, every variant; the private-state snapshot includes the review history) |
| Forced RLS, append-only, guards | `TestTripReviewSchemaIsForcedRLSAppendOnlyAndGuarded` (unsourced/in-progress SUBMIT, opening a draft, self review, skipped opening, wrong state, reasonless RETURN, unaudited decision at commit, UPDATE/DELETE/TRUNCATE 55000 with privileges, nothing visible or insertable without the organization) |
| Populated upgrade from the 000224 head | `TestTripReviewPopulatedUpgradeFailureRecoveryAndPreviousImage` (completed DRAFT trip; injected failure leaves schema/data; recovery once; no-op replay; previous image compatible; forced-RLS migrator ownership; the pre-existing trip then submitted and approved) |
| Contract | `TestTripReviewContractOwnerShapesAndGuards`, owner count in `TestMeterStandaloneContractHasOneExplicitAtomicOwner`, inventory and full-router tests in `cmd/api` |

## Merge with the integration head

Task commit `95d5555e` on `0fb49112`; then, as the coordinator asked, a
normal merge of `codex/enterprise-f4` at `08b3e37c` (EMP-CHECK-001,
migration 000223) in one merge commit (validated as `d4727b3c`; only this
evidence file was amended into it afterwards). Conflicts were resolved as
unions: the contract sections (CHECK-001 then TRIP-004), the company
authority policy allow-list, the suite audit definitions, the
check-manifest, the full-router path list and the matrix representatives;
the OpenAPI trip blocks were re-inserted structurally on the integration
head's file (503 added lines, no line of the head changed). 000223
redefines `org_asset_current_usage`; 000226 redefines no usage source, so
nothing had to be rebuilt. Counts on the merged tree: 134 normal routes
(routed 24, signed 1, not routed 60, specials 25), AUDITOR walk 134 routes /
30 positive / 104 FORBIDDEN, authorization matrix 240 cells (93 allowed /
147 denied).

## Validation

All gates pass on the merged tree (validated as `d4727b3c`: task commit
`95d5555e` plus the integration head `08b3e37c`). Go 1.27.4 toolchain from `backend`,
`GOCACHE=/data/dev/cache/f4-go-build`, real PostGIS through `orgtest`
isolated child databases, the coordinator MinIO (`127.0.0.1:32819`,
`validation-private`) for the storage-dependent suites:

```sh
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/{trip,organization,review}/... ./internal/platform/{enterprisesuite,apicontract}/... ./cmd/api/...   # clean
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/trip/... ./internal/modules/organization/adapters/... ./internal/platform/apicontract/... ./cmd/api/...   # clean
sqlc vet && sqlc diff                                                 # clean (sqlc generate reproduces db/queries/trip incl. reviews.sql.go)
go test -json -race -tags=integration -count=1 -p 2 -timeout=60m ./internal/modules/organization/adapters ./internal/modules/{trip,review,fleet,orgsync,orgaccess,maintenance}/... ./internal/platform/{enterprisesuite,apicontract,migrate,orgtest,orgapi}/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root
python3 docs/enterprise/tools/check_plan.py                           # repo root
git diff --check
```

- Race/integration union, log `trip004-db-union.jsonl` (27 min 7 s;
  `organization/adapters` 1494 s): 33 packages, 32 pass, 1 without tests
  (`orgtest`), 1054 top-level / 9336 named tests pass, 0 failures, 0 skips.
  It includes every `TestTripSubmit*`/`TestTripReview*` test, the matrix at
  240 cells (93/147), the AUDITOR walk at 134/30/104, the CHECK-001 and
  TRIP-002 suites, `migrate` on the full chain and the storage-dependent
  document/export/auditor suites with MinIO. The 5 `cmd/worker` tests that
  need the pinned Rust preparer CLIs are outside this union (untouched).
- Untagged whole module, log `trip004-untagged-all.log`: exit 0, 222
  packages ok, 80 without tests, 0 failures (3 min).
- Static gates, log `trip004-merged-static.log`: all clean.
- Unit coverage (`trip004-go-cover-unit.out`, race): `trip/domain` 100.0%,
  `trip/application` 66.6%, `trip/adapters/http` 37.1%, `trip/adapters`
  8.0%; the store, handler and route paths are exercised by the PostGIS
  tests above.
- OpenAPI: 0 vacuum errors; 1018 warnings / 660 informs versus 1006 / 659 on
  the integration head (the existing enterprise path-pattern and
  description-style rules), log `trip004-vacuum.log`.
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean.
- Secret/PII review of the task diff: the pattern scan hits only synthetic
  test fixtures (session access tokens of synthetic accounts, a repeated-byte
  cursor secret) and a context line of EDR-217; `gitleaks stdin --redact`
  finds no leaks. Synthetic data only.
- No Kotlin, Android, shared vector, job or deployment change, so no Gradle
  run applies; no emulator or device was used.

## Retained obligations

- EMP-TRIP-015: the author's correction loop after RETURNED (a new revision
  through events/amendments and `RESUBMIT`), late and duplicate submissions
  and their held review (`STALE_REVISION`), void and amendments of approved
  trips; until then a non-draft submission stays `RETRY`/`UNAVAILABLE` and a
  submission whose right changed after its declaration stores nothing.
- EMP-TRIP-005: the Android producer (queue `trip.submit` behind the finish,
  "ready to send" until the ACK) and the author's own review-result read
  (VAL-01 "Bruno's app shows both outcomes"); no own-result route exists.
- EMP-TRIP-006: SCR-TRIP-02, the timeline, lists filtered by structural
  scope (today the queue is organization-level and ORG grants only) and the
  auditor's trip and review read under `trip.trip.read`.
- EMP-TRIP-016: period lock of trip decisions (the APPR-006 gate is not
  consulted here); EMP-TRIP-020 the closing conference before submission;
  EMP-APPR-002/003 multi-level policies and substitutes.
- Author cancel (`CANCEL`) is not offered; TEAM-scoped review is resolved by
  the `TripStructure` source (author/driver team links at the start receipt)
  but proven only by construction and the shared EDR-161 matching, not by a
  dedicated PostGIS team fixture.
- Merge note for the coordinator: the upgrade test builds the TRIP-002
  fixture on an older image through `tripState.handlers`; if EMP-TRIP-003's
  wiring adds ports to `tripState`, `tripStateOn` must set them too.
- Production runtime grants (not provisioned here): `SELECT, INSERT ON
  org_trip_review_entries`; `SELECT ON org_trips, org_trip_finishes,
  org_team_links, org_memberships, org_employee_identities,
  org_audit_events, org_sync_operations, org_sync_acknowledgements,
  org_sync_effects, org_device_enrollments` for the guards and the
  structure source; the migrator needs `REFERENCES ON org_trips,
  org_memberships, org_employee_identities`. Proposed README text is this
  paragraph.
- No C1, M2, M4 or F4 acceptance, deployment or real data is claimed.
