# EMP-TRIP-012 — Driver handover during a trip

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task on `codex/enterprise-f4-trip012` (worktree
`/data/dev/worktrees/f4-trip012`). Started on the F4 head `4f9170fc`
(TRIP-001..005, TRIP-007/008/009/011/013/015/016, GPS-001, OFFL-005,
CHECK-001/003); on the coordinator's instruction the uncommitted work was
re-applied on the F4 heads `b4249772` (TRIP-014/018, migrations 000235/000236,
PR #225 with 000239), `0912b48a` (GPS-004/012, migration 000240) and
`2f031276` (EXP-002, migration 000244, which redefines only the usage
functions and sorts after 000243) by fast-forwarding the branch and
re-applying the working tree (backups of every working state:
`f4-logs/trip012-wip-*.patch` and `*-untracked*.tgz`); every redefinition
was rebuilt from the latest bodies, and all final gates ran on `2f031276`
plus this change. The commit's parent is `2f031276`. The coordinator
integrates the commit and owns PROGRESS, the F4 ledger, pushes and the PR. Synthetic data only; no Android app/data module, panel, job,
deployment or device change.

Read: AGENTS, FAST_EXECUTION, 00-conventions; the EMP-TRIP-012 block and the
TRIP-002/003/004/007/011/014/015/018/021 blocks ([EP06](../backlog/ep06-trips.md));
[trips](../modules/trips.md) (UC-TRIP-03, SCR-TRIP-03, "an ended membership
triggers handover"); REQ-07.6 and REQ-06.3 in
[17-traceability](../17-traceability.md); BR-TEN-003, BR-AUD-001/004/005,
BR-TIME-002/003/005, BR-REV-003/007 ([04-business-rules](../04-business-rules.md));
the `trip.trip.close_for_other`, `trip.trip.start` and `trip.trip.review` rows
of [07-permissions](../07-permissions.md); [08-state-model](../08-state-model.md)
§4 rule 7; the offline matrix and BR-OFFL-004/006 of
[10-offline-sync](../10-offline-sync.md); EDR-177/178, 202..209, 220..223,
234..237, 252/253, 257..264, 271..281 of [16-decisions](../16-decisions.md);
the [F4 ledger](f4-execution.md) and the evidence of TRIP-002, TRIP-003,
TRIP-004, TRIP-007, TRIP-011, TRIP-014, TRIP-015, TRIP-018, GPS-001 (its
retained session-ownership question) and GPS-004; the coordinator's design
notes for TRIP-012 (followed: supervisor-initiated online route with fresh
`trip.trip.close_for_other`, CAS on the trip revision, server instant,
eligible assigned receiver, insert-only handovers counted in the revision,
lifecycle/finish/submit by the current responsible member, events by the
member responsible at the declared instant, redefinitions from the latest
bodies, review separation of every handover party, races of two handovers
and of a handover against a pause).

IDs used: B-BR-EMP-394..398, BUC-EMP-199..201, EDR-254..256 — all of them.
Migration `000243_organization_trip_handovers.sql` (the reserved 000234 would
sort before the 000233/000237 bodies it extends and stays unused).

## Rules frozen before behaviour

Rules B-BR-EMP-394 (supervisor's online handover, eligibility, outcomes, pure
order), B-BR-EMP-395 (future actions use the new responsibility; history keeps
its actors), B-BR-EMP-396 (one revision, one responsible member), B-BR-EMP-397
(separation: recorder, review and closure) and B-BR-EMP-398 (the handover
list, the receiver's own trips, tracking ownership) and use cases
BUC-EMP-199..201 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-012 — Driver handover during a trip (2026-11-15)". Decisions, the
recommended routine choices taken unattended, are in
[16-decisions](../16-decisions.md):

- EDR-254: the handover is a fact of the pure trip in Go and Kotlin over a new
  shared `trip/handover-v1.psv`; `Responsible()` (current) gates pause, resume,
  cancel and finish; `ResponsibleAt(t)` gates logbook events, so the giving
  driver's offline facts declared before the handover still arrive; the
  instant is the server's and is ordered only against earlier handovers; no
  new `TripFailure` token (the trip-v2 coverage stays closed).
- EDR-255: persistence, derived responsibility (no trip column moves), the
  twelve redefinitions, routes, eligibility, intake alignment and counts.
- EDR-256: tracking session ownership after a handover (the session stays the
  opener's; the old owner's device ends its session, the receiver opens its
  own; the tracking owner must add the end cause and relax one-session-per-
  trip; the server trip facts already follow the responsibility).

Design answers to the task's open points:

- **Who and how.** `POST /v1/orgs/{org_id}/trips/{trip_id}/handover` under
  fresh `trip.trip.close_for_other` covering the trip's stored structure
  (`TripStructure`), ACTIVE organization, body `schema_version`,
  `operation_id`, `revision` (CAS), `receiver_membership_id`, `reason`; the
  server clock is the instant (BR-TIME-003). Online only: no signed device
  operation (C1 stays driver-initiated) and no device-side decision.
- **Eligible receiver.** ACTIVE membership, ACTIVE unmerged employee record,
  current `trip.trip.start` covering the trip as its own record and a
  FLEET-006 assignment of the trip's asset at the database instant, not the
  current responsible member; anything else (unknown, foreign, suspended,
  unassigned, no permission) is one `VALIDATION_FAILED` without detail.
- **Separation.** The recorder is neither the current responsible member nor
  the receiver (account, membership, canonical employee): `SELF_APPROVAL`.
  Every handover party is excluded from every review decision of the trip;
  the current responsible member never closes the trip for another.
- **Future actions vs history.** Pause/resume/cancel/finish/submit by the
  current responsible member; events, incident reports inside the trip and
  the tracking trip facts by the member responsible at the declared instant;
  nothing stored is rewritten; a former responsible member re-reads its own
  receipts.
- **TRIP-014/018 interplay.** A closed trip takes no handover; the closer must
  not be the current responsible member (application, domain, closure guard);
  offboarding open work (`TRIP:<id>`) is the trips the ending membership is
  currently responsible for (open-work read and guard).
- **Kotlin.** The receiver's device must rebuild a handed-over trip in its own
  pure domain (EMP-TRIP-021), so `Trip.handover`, `responsible`,
  `responsibleAt` and `handovers` are mirrored with the shared oracle; no app,
  data or DI module changes.

## Shape

- Domain (Go `trip/domain/handover.go`; `pause.go` `Responsible`, `logbook.go`
  `Log` at the declared instant, one `trip.go` field; Kotlin
  `TripHandover.kt`, `Trip.kt`, `TripLogbook.kt`): `HandoverInput`,
  `HandoverFact`, `Trip.Handover`, `Handovers`, `ResponsibleAt`.
- Oracle `contracts/testdata/enterprise/trip/handover-v1.psv` (87 cases:
  HANDOVER 47, RESPONSIBLE 12, ACT 28; SHA-256
  `5dd11b0510c63fc8081e6c7a4b743e2a5af580a03ec233092cc3573768bd456a`),
  generated by `f4-logs/trip012-scripts/gen_handover_v1.py` from the written
  rules; documented in the trip testdata README.
- Application `trip/application/handover.go`: policies, `HandoverCommand`
  (digest tag `trip.handover`), records, results, pages, `HandoverService`
  (live decision before and after the store, result re-proof),
  `HandoverDomain`, the replay step, and `Record.Responsible`,
  `ResponsibleAt`, `ResponsibleBefore`, `Driver`, `DriverAt`, `Party`,
  `HandoverParties`; `Record.Handovers` (one field), `Record.Revision` counts
  handovers, `replayDomain` merges handovers by revision, `DecideRevision`
  takes other excluded people (variadic, the EMP-TRIP-014 form).
- Adapters `trip/adapters/handover_store.go` (authority holding the root →
  trip row lock → replay by operation → CAS → separation → eligibility →
  pure handover → row + `trip.trip.handed_over` audit event → authority
  again; the handover list and own-trips list), `ReadTripTx` reads and
  validates the merged revision sequence (`Store.PreHandoverImage` for
  rehearsals before 000243, the TRIP-011 precedent), handlers use
  `tripTarget` (responsible member; a former responsible member reads its own
  receipts), `Driver`/`DriverAt` for the history rule and the responsible
  employee for stored-fact proofs (finish, events, lifecycle, submit, late
  finish), the review store excludes handover parties, the closure store
  excludes the current responsible member; the trip open-work query
  (`OpenTripsOfAuthor`) follows the responsibility.
- Consumers: `maintenance/application.TripWindow` (incident report inside a
  trip) and `tracking/application.TripFactsAt` (EDR-279) use
  `ResponsibleAt`.
- Authority `organization/adapters/trip_handover_authority.go`
  (`TripHandoverAuthority`: the trip review company proof for
  `close_for_other`, an own-target proof for `trip.trip.start`, receiver
  eligibility through role candidates and the FLEET-006 port);
  `company_authority.go` admits the two new policy values;
  `resource_structure.go` resolves `TripStructure` for `close_for_other`.
- HTTP `trip/adapters/http/handover.go` (three routes, strict bodies,
  `handoverMapped`); `enterprisesuite/routes.go` wires the store, the audit
  definition and widens the `trip.close` admission rationale.
- Migration 000243 (generator `f4-logs/trip012-scripts/gen_migration_243.py`,
  which locates each latest body by scanning the migrations before 000243 and
  asserts its source file and every replaced line): forced-RLS insert-only
  `org_trip_handovers` (PK organization/trip/revision, unique operation,
  composite keys to the trip, three memberships and employees, deferred key to
  the audit event, a parties check), `org_trip_responsible`,
  `org_trip_responsible_member` (SET clauses, never inlined), the handover
  source and deferred publication guard; twelve redefinitions keeping every
  other line (233: transition, held-transition, finish sources; 235:
  revision, logbook, held-logbook, review-entry, closure sources; 236: held
  source, offboarding guard; 229: held-submission source; 230: incident
  report source). The line-preservation check (`trip012-migration-lines.log`)
  shows only the declared lines changed.
- Contract and inventories: OpenAPI 3 paths and 7 schemas; inventory
  `trip.trip.close_for_other` 4 routes, `trip.trip.start` 2 routes; router
  path list +3; counts on `0912b48a`/`2f031276` (EXP-002 adds no route): 156 normal routes (30 routed / 54 not
  routed / 26 specials), AUTH-011 walk 156 / 31 / 125, matrix unchanged at 300
  cells (121 / 179; `close_for_other` keeps the EMP-TRIP-014 open-list
  representative); new `apicontract` `TestTripHandoverContractShapesAndGuards`.
- Fixtures: `tripHandoverGrant` (conditional on 000243) is applied by
  `tripGrants`, `tripReviewGrants` and the 235/236 rehearsals after the
  remaining chain; the trip store and the review store built on older images
  set `PreHandoverImage` (the TRIP-011 `PreLifecycleImage` precedent:
  `tripFixtureOn`, `tripStateOn`, `tripReviewStoreOf` and the 233 rehearsal
  read it from the image; production values are the zero values).

## RED

Logs in `/data/dev/worktrees/f4-logs/trip012-*`. Shape first (rules, EDRs,
exported names, Go stubs `errNotImplemented`/start author, Kotlin `TODO()`,
the frozen oracle and tests, the migration as written), then:

- Go domain (`trip012-go-domain-red.jsonl`): 31 top-level, 5 failing (the
  vector test and the four handover unit tests); 81 of 87 vectors failing.
  The 6 passing rows are owned by existing code: three fixture-level
  construction refusals (`handover-malformed-at`, `-malformed-receiver`,
  `-malformed-recorder`) and three rows without any handover
  (`responsible-without-handover`, `-without-handover-at`,
  `act-starter-pause-without-handover`).
- Kotlin (`trip012-kotlin-red.log`, XML `trip012-kotlin-red-xml`): 954 trip
  tests, 83 failing (81 vectors and both unit tests); the same 6 rows pass.
- PostGIS on the base `4f9170fc` (`trip012-db-red.jsonl`, `-race
  -tags=integration -p 2`, the store entry points stubbed `ErrUnavailable`
  and the intake handlers not yet responsibility-aware): 7/7 top-level and
  4/4 subtests failing, each at the stubbed store (the four race subtests
  because the stub never takes the root lock); the upgrade rehearsal passed
  every migration assertion (injected failure, recovery, kept lines) before
  failing at the stubbed handover.
- The application rules and the store were written after the domain RED and
  before their PostGIS GREEN; the HTTP route test and the apicontract test
  were written with the routes and are not claimed as RED.

## GREEN and REFACTOR

- Go domain (`trip012-go-domain-green.jsonl` on the base,
  `trip012-go-domain-green-merged.jsonl` on `0912b48a`, `-race`): 31/31
  top-level on the base, 36/36 top-level and 896 named subtests on the merged
  tree (with the EMP-TRIP-014 closure tests), 87/87 vectors; coverage 99.9% (the
  only uncovered statement is the EMP-TRIP-014 closure's unreachable
  `UNSUPPORTED_COMMAND` branch; every handover statement is covered). One
  test was added on the merged tree: `TestHandedOverTripIsClosedOnlyByAThirdMember`.
- Kotlin: trip tests 954/954 (`trip012-kotlin-green.log`); the enterprise
  domain suite on `0912b48a` 63 suites / 4183 tests, 0 failures
  (`trip012-kotlin-enterprise.log`, XML `trip012-kotlin-enterprise-xml`).
- PostGIS, first GREEN run on `0912b48a` (`trip012-db-green1.jsonl`, `-race
  -tags=integration -p 2`, the handover, closure, offboarding, late-intake,
  lifecycle, review, event, intake, submit and tracking tests): 56 top-level
  and 75 subtests; all 8 handover tests and their 4 race subtests passed on
  the first run (no store or handler defect surfaced); one failure, the
  EMP-TRIP-014 closure upgrade rehearsal, was a fixture gap: after applying
  the remaining embedded chain (now including 000243) it regranted only the
  closure table, so its closure (which reads the revision and responsibility)
  met `42501`; the 235 and 236 rehearsals now also call `tripHandoverGrant`
  after the chain (no expectation changed).
- Unit packages touched (`trip012-unit-merged.jsonl`, `-race`): trip,
  tracking and maintenance, 139 top-level / 1665 subtests, 0 failures.
- REFACTOR: the migration generator was rebuilt to locate each latest body by
  scanning every migration before 000243 and to assert its source file (it
  caught the moves to 000235/000236 when the F4 head advanced); the duplicate
  `sharesEmployee` was dropped in favour of the EMP-TRIP-014 helper.

| Requirement | Test(s) |
|---|---|
| AC: future actions use the new authorized responsibility | `TestTripHandoverMovesFutureResponsibilityAndKeepsHistory` (receiver pauses at rev 3, records a stop, resumes at rev 4, finishes at rev 5 with 184 610 000 m, submits; the own list shows the trip with its handover, then nothing once finished); upgrade rehearsal (a pre-upgrade paused trip handed over, resumed and finished); vectors `act-receiver-*`, `act-paused-handover-*`, `act-chain-*` |
| AC: existing events retain their original actor | same test (start, both pre-handover events, lifecycle rows and finish keep their authors in the database and in the timeline; the giving driver's offline NOTE declared before the handover arrives under its own author; its receipts replay byte for byte); vectors `act-giver-log-before-handover`, `act-giver-late-log-before-handover-after-finish`, `responsible-*` |
| Foreign driver | `TestTripHandoverRefusesIneligibleClosedStaleSelfAndUncovered` (another organization's membership, an unknown id, an unassigned driver, a member without `trip.trip.start`, a suspended member, the current responsible member: `VALIDATION_FAILED`, nothing written); HTTP unknown receiver 422; domain `SCOPE` vectors for foreign references |
| Closed trip | refusal test (completed and cancelled trips `CONFLICT`); `TestTripHandoverRespectsClosureAndOffboarding` (a supervisor-closed trip `CONFLICT`); schema test (guard `trip already closed`); vectors `handover-completed`, `-cancelled` |
| Stale handover (revision CAS) | refusal test (future revision, revision before a pause); main test (`again` replays, another digest `OPERATION_ID_REUSED`); HTTP 409; schema test (forged and stale revisions refused); vectors `handover-stale-*`, `handover-again-is-stale` |
| Overlapping ownership from concurrent commands | `TestTripHandoverConcurrentCommandsResolveToOne/{handover-first-of-two,handover-second-of-two,handover-then-pause,pause-then-handover}` (barrier on the organization root, both orders observed waiting; exactly one handover or one pause applies; one responsible member; a pause after the handover is `FORBIDDEN` with nothing stored, a handover after the pause `CONFLICT`) |
| Self-review afterwards | `TestTripHandoverPartiesNeverReviewTheTrip` (giving member, receiver and recorder each refused APPROVE, RETURN and REJECT with `SELF_APPROVAL`; the review-entry guard refuses a direct entry by the recorder; an independent reviewer decides); main test (recorder refused, independent approver approves) |
| Separation of the recorder; closure interplay | refusal test (an ADMIN handing a trip to itself, the current responsible member handing its own trip over: `SELF_APPROVAL`); closure/offboarding test (the receiver cannot close; a third supervisor can); HTTP 409 `SELF_APPROVAL`; domain test and vectors `handover-recorder-*` |
| Offboarding flags follow the responsibility | `TestTripHandoverRespectsClosureAndOffboarding` (the giver's offboarding does not list the handed-over trip and completes; the receiver's lists it, is refused without it and keeps the trip open) |
| Authority and isolation | refusal test (AUDITOR, DRIVER, APPROVER `FORBIDDEN`; a supervisor of another unit `FORBIDDEN`; a covering UNIT supervisor hands a paused trip over; another organization's trip and an unknown trip `NOT_FOUND`); HTTP AUDITOR 403 on the three routes, foreign organization 404; AUTH-011 walk |
| Database guards, forced RLS, insert-only | `TestTripHandoverSchemaIsForcedRLSGuardedAndInsertOnly` (forged/stale revision, giving member not responsible, employee mismatch, recorder party, receiver = giver, suspended receiver, missing audit event at commit, UPDATE/DELETE/TRUNCATE, no trip column moved, nothing visible without the organization) |
| Populated upgrade 000240 head → 000243 | `TestTripHandoverPopulatedUpgradeFailureRecoveryAndPreviousImage` (completed trip, paused trip with an event, held busy start; injected failure leaves schema, the twelve functions, constraints, indexes, data and revisions; ordinary migrator; recovery once; no-op replay; previous image compatible; kept lines of every redefinition; receipts replay; revisions unchanged; handover, resume and finish on the upgraded schema; remaining chain) |
| Wire contract | `TestTripHandoverThroughComposedCompanyHTTP`, `TestTripHandoverContractShapesAndGuards`, router path list and inventory counts |


## Validation

Go from `backend` (toolchain go1.27.4, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`, `-p 2`), real PostGIS through
`orgtest` child databases (NOSUPERUSER/NOBYPASSRLS runtime role), a
throwaway MinIO project `f4-trip012-s3` (`infra/compose.validation.yml`,
`storage` plus the `init` profile; removed with `down -v`):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/{trip,tracking,maintenance,organization}/... ./internal/platform/{enterprisesuite,apicontract}/... ./cmd/api/...   # clean
sqlc vet && sqlc generate && sqlc diff                               # clean (handovers.sql new, closures.sql open-work query)
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml   # 0 errors
go test -json -race -tags=integration -timeout=150m -p 2 -count=1 ./internal/modules/organization/adapters ./internal/modules/{trip,tracking,fleet,orgsync,review,maintenance,membership,expense}/... ./internal/platform/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' (flock)
python3 docs/enterprise/tools/check_plan.py
git diff --check
```

Results on the final tree (`2f031276` + this change):

- Race/integration union (`trip012-db-union.jsonl`, exit 0, 28 min,
  `organization/adapters` 1,588 s): 55 packages, 51 pass and 4 have no test
  files; 1,476 top-level and 10,379 named subtests pass; 0 failures, 0
  skips. It includes every handover test, every `Upgrade` rehearsal, the
  closure, membership-end, tracking, expense, review, amendment, period,
  conflict, lifecycle, mission, event and incident suites, the authorization
  matrix (300 cells, 121 / 179), the AUTH-011 walk (156 / 31 / 125) and the
  storage-backed suites with the throwaway MinIO.
- The first union on `0912b48a` (`trip012-db-union-0912b48a.jsonl`, 60
  min, 51 packages, 1,420 top-level / 9,762 subtests passed) failed two
  rehearsals that populate images before 000228/000226 with the review
  store, whose decision now reads the handover parties from a table their
  image lacks; the review store got the `PreHandoverImage` knob set by
  `tripReviewStoreOf` from the image (no expectation changed). A focused
  rerun on `0912b48a` (`trip012-db-rerun.jsonl`, 57 top-level passed
  including the amendment rehearsal) was stopped by me when the F4 head
  advanced, and the final union above covers it.
- Untagged whole module (`trip012-untagged-all.log`, exit 0): 229 packages
  pass, 87 have no test files, 0 failures.
- Kotlin enterprise domain suite on the final tree: 65 suites / 4,227 tests,
  0 failures (`trip012-kotlin-enterprise-final.log`).
- `cmd/api` inventory/router tests and `apicontract` handover, closure and
  membership-end contract tests pass untagged.
- Static: toolchain gofmt empty; build and vet (both tags) clean;
  staticcheck plain and integration clean (`trip012-staticcheck-final.log`);
  `sqlc vet`/`sqlc diff` clean; vacuum 0 errors with 1,118 warnings / 681
  informs versus 1,109 / 681 on the head (the existing enterprise
  path-pattern and description-style rules on the three new routes);
  `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean.
- Migration line preservation (`trip012-migration-lines.log`): each of the
  twelve redefinitions starts from its latest body and changes only its
  declared lines (0–3 per function).
- Secret/PII review of the diff: synthetic identifiers, names and texts only;
  the disposable MinIO and PostGIS credentials appear only in the
  uncommitted log environment files.
- Not run: the 15 Rust-CLI-gated integration tests of untouched `cmd/worker`,
  `directory` and `quality` (outside the union, per the brief); no emulator
  or device was used.

## Retained obligations

- **Tracking session handover (EDR-256)** for EMP-GPS-002/004: an end cause
  for a handover, one open session per trip with at most one per owner, the
  000240 opening guard (owner = trip author) and the Android service ending
  the giving device's session; until then the remaining trip is untracked
  (an explicit gap). The GPS-012 recorded-track read still resolves the
  trip's starter as the driver of the own-record rule.
- **Android (EMP-TRIP-021)**: a signed device feed of handed-over trips, the
  trip book importing them and the giving device reacting to `FORBIDDEN`;
  the Kotlin domain is ready.
- **Panel (EMP-TRIP-006/022, SCR-TRIP-03)**: the handover in the trip
  timeline and the review decision context (today the timeline shows every
  fact with its actor and the handover list shows the handover); the
  EMP-TRIP-014 open-trip list still names the starting member and its state
  (its `author_*` fields), the current responsible member is in the
  handover list.
- **Mission link (EMP-TRIP-023)**: the mission's running-trip check still
  matches the start driver.
- A late finish of a member that lost the trip through a handover is
  `MALFORMED` like any other member's; its reading can be recorded by the
  receiver or through an amendment.
- Clock skew: a device start declared after the server handover instant
  makes the giving driver's later events `FORBIDDEN`; BR-TIME-005 flags the
  gap, no ordering against device instants is imposed (EDR-254).
- Production grants (README "Trip driver handover"), notifications to the
  receiver, and no C1, M4 or F4 acceptance, deployment or real data is
  claimed.
