# EMP-TRIP-001 — Pure trip domain: operational states and the driver-initiated trip

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 pure-domain task in worktree branch
`codex/enterprise-f4-trip001` (based on `origin/dev` 8c2ee654); the
coordinator integrates it into `codex/enterprise-f4` and owns PROGRESS, the
F4 execution ledger, pushes and the PR. This task adds a pure Go package
`backend/internal/modules/trip/domain`, its Kotlin mirror
`com.anpfuel.domain.enterprise.trip` (the Android app will execute trips
offline in EMP-TRIP-005) and one shared oracle
`contracts/testdata/enterprise/trip/trip-v1.psv`. No migration, SQL, route,
OpenAPI, Android app/data module, DI, persistence, job, producer or
dependency is added. Synthetic data only.

Rules read: the EMP-TRIP-001 and EMP-TRIP-002..023 task blocks
([EP06](../backlog/ep06-trips.md)); the [trip design](../modules/trips.md)
(BR-TRIP-001..004, UC-TRIP-01..03, §§6–10 and the 184 320 000 m →
184 610 000 m example); the [state catalog](../08-state-model.md) §§1, 3, 4
(three independent kinds of state, the seven trip operational states,
review = BR-REV plus VOIDED, terminal states and closed lists);
BR-REV-001/002, BR-TEN-001/004/005/006, BR-TIME-001..005, BR-MONEY-002
([shared rules](../04-business-rules.md)); REQ-07.1/07.4/07.5/07.6
([traceability](../17-traceability.md)); the offline matrix rows "Start
trip" and "Finish trip" and the intake codes `ASSET_BUSY`,
`TRIP_ALREADY_CLOSED` ([offline sync](../10-offline-sync.md)); the trip
permissions `trip.trip.start` (SELF) and `trip.trip.close_for_other`
([permissions](../07-permissions.md)); VAL-01 and VAL-07
([validation](../15-validation.md)); EDR-028, EDR-094, EDR-128..133 and
EDR-173..177 ([decisions](../16-decisions.md)). Precedents copied:
EMP-METER-001, EMP-FUEL-001, EMP-EXP-001 and EMP-APPR-001 (structure, oracle
grammar, hash assertions and redaction).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-268..275, BUC-EMP-136..138, EDR-178..181. All
are used; none remains reserved.

Rules B-BR-EMP-268, B-BR-EMP-269, B-BR-EMP-270, B-BR-EMP-271,
B-BR-EMP-272, B-BR-EMP-273, B-BR-EMP-274, B-BR-EMP-275, BUC-EMP-136,
BUC-EMP-137 and BUC-EMP-138 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-001 — Trip domain core: operational states and the
driver-initiated trip (2026-11-15)", written before any behaviour.

Decisions EDR-178 (C1 lifecycle; pause, resume and cancel refused as
`UNSUPPORTED_COMMAND` until EMP-TRIP-011), EDR-179 (eligible context as a
complete same-organization supply with an ordered `INCOMPLETE` list; only
the starting member finishes in C1), EDR-180 (reading reference plus exact
value, usage never invented, finish never refused for plausibility) and
EDR-181 (placement, own references, closed review input without importing
`review`, no delivery state in the domain) are recorded in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

What PAUSED and CANCELLED mean here: both stay in the closed catalog
(`ParseState` accepts them, CANCELLED is terminal) but no C1 command
produces or leaves them. PAUSE, RESUME and CANCEL are recognized commands
refused with `UNSUPPORTED_COMMAND`; any command on PLANNED, ASSIGNED,
ACCEPTED or PAUSED is also `UNSUPPORTED_COMMAND`, because missions, planned
trips and pause/resume/cancellation belong to EMP-TRIP-008/009/011/021.
Any command on COMPLETED or CANCELLED is `INVALID_TRANSITION` first:
terminal states stay terminal and a new attempt is a new trip.

## Shape frozen before behaviour

Go package `backend/internal/modules/trip/domain`: errors `ErrInvalid`,
`ErrScope`, `ErrIncomplete` (with `IncompleteError.Missing()`),
`ErrForbidden`, `ErrInvalidTransition`, `ErrUnsupportedCommand`,
`ErrFinishBeforeStart`; `State` (seven constants, `ParseState`,
`Terminal`), `Command` (`CommandStart`..`CommandCancel`, `ParseCommand`),
`Transition`; `ReviewState` (eight constants, `ParseReviewState`),
`Standing` (`NewStanding`, `Completed`, `Approved`); `TripRef`,
`EmployeeRef`, `MembershipRef` with constructors; `MeterObservation`
(`ObservedReading` taking the METER-001 `meter.ReadingRef` and
`meter.Value`, `AbsentReading` taking `meter.Missing`); `Missing`;
`StartInput`, `FinishInput`, `Event`, `Trip` (`Start`, `Finish`,
accessors, `MeterUsage`) and `MeterUsage` (`UsageStatus`, `UsageReason`).
Private values are redacted in fmt, slog and JSON like FUEL-001/EXP-001.

Kotlin mirror `com.anpfuel.domain.enterprise.trip`: `TripFailure`,
`TripMissing`, `TripDomainException` (with `missing`), `TripState`,
`TripCommand`, `TripLifecycle.transition`, `TripReviewState`,
`TripStanding`, `TripRef`/`EmployeeRef`/`MembershipRef`,
`TripMeterEvidence.Observed`/`Absent` (meter `ReadingRef`, `MeterValue`,
`MeterPresence.Missing`), `TripStartInput`, `TripFinishInput`, `TripEvent`,
`Trip.start`/`finish`/`meterUsage` and `TripMeterUsage`; every `toString` is
redacted.

Reuse: the start declaration uses `orgtime.ValidateDeclaration` /
`EnterpriseTime.validateDeclaration` and the start reporting day the
EXP-001 receipt-free `orgtime.AttributeReportingDay` /
`EnterpriseTime.attributeReportingDay`; asset references are the FLEET-001
`fleet.AssetRef`; readings, values and missing reasons are METER-001 types
(no new meter value or reason profile). No kernel changed. `orgvalue` is not
used because no amount appears. The `review`, `fueling` and `expense`
packages are not imported; the review vocabulary is a closed input enum.
`scripts/check-manifest.txt` lists `backend/internal/modules/trip/domain` in
`[quick-packages]`.

## RED

Both languages first compiled against exported stubs (Go zero values and
`errNotImplemented`, Kotlin `TODO()`), with the frozen oracle and tests.

- Go (`go test -race -count=1 -json`), log
  `/data/dev/worktrees/f4-logs/trip001-go-red.jsonl`: 10 top-level tests,
  9 failing, and 207/207 shared-vector subtests failing (package 0.049 s).
  The one passing top-level test exercises only the fixture grammar reader.
  Zero skips.
- Kotlin (`:domain:test --tests 'com.anpfuel.domain.enterprise.trip.*'`,
  flock). A first draft (log `trip001-kotlin-red-draft1.log`, XML in
  `trip001-kotlin-red-draft1-xml/`) had 217 tests and 209 failures: the
  grammar test and seven vectors passed for the wrong reason, because the
  Kotlin reference stubs accepted any identity, so the fixture reached the
  accepted meter, fleet and time constructors, which refuse those inputs
  (blank, empty, control and 281-scalar missing reasons, a value above the
  odometer bound, a malformed instant, a malformed organization of the
  fixture asset). The Go stubs refused every trip reference. The Kotlin
  reference stub now has the same unimplemented constructor
  (`init { TODO() }`); no expectation changed. Rerun, log
  `/data/dev/worktrees/f4-logs/trip001-kotlin-red.log`, XML in
  `trip001-kotlin-red-xml/`: 217 tests, 216 failures, 0 errors, 0 skipped
  (unit 9/9 failing in 0.272 s, vectors 207/208 failing in 0.451 s; only
  the grammar method passes; Gradle 16 s). Those seven rows remain owned by
  the accepted kernels in GREEN (the trip types add their own zero-value and
  modulus checks, covered by unit tests).

## GREEN and REFACTOR

- Go, log `/data/dev/worktrees/f4-logs/trip001-go-green.jsonl` and coverage
  profile `/data/dev/worktrees/f4-logs/trip001-go-green-coverage.out`: the
  first implementation passed 10/10 top-level and 207/207 subtests (race,
  atomic coverage, 1.058 s), zero failures and skips; statement coverage
  100.0%.
- Kotlin targeted, log `/data/dev/worktrees/f4-logs/trip001-kotlin-green.log`:
  217 tests (vectors 207 + grammar, unit 9) pass, 0 failures, errors or
  skips (Gradle 22 s). The compiler reports no warning in the trip sources.
- REFACTOR: the Go `ObservedReading` shape check was reduced to the only
  reachable invalid reference (the zero value; a constructed METER-001
  reference is always fully scoped) and the reference `validate` comment now
  states that callers check the scope first; tests rerun green with 100.0%
  statements. staticcheck (plain and `-tags=integration`) is clean.
- Kotlin: coverage review showed the data-class default constructor of
  `TripStartInput` unexecuted, so the absent-context unit test also starts
  with `TripStartInput()` (`INVALID`, the identity is absent); it passed on
  first run and is not claimed as RED.
- Kotlin final whole enterprise suite (`--tests
  'com.anpfuel.domain.enterprise.*'`), log
  `/data/dev/worktrees/f4-logs/trip001-kotlin-enterprise-final.log`, XML in
  `/data/dev/worktrees/f4-logs/trip001-kotlin-final-xml/`: 40 suites, 2502
  tests, 0 failures, 0 errors, 0 skipped (Gradle 31 s), including trip 217
  (vectors 208, unit 9). JaCoCo
  (`/data/dev/worktrees/f4-logs/trip001-kotlin-jacoco.xml`) for package
  `com.anpfuel.domain.enterprise.trip`: instructions 1436/1442 (99.58%),
  branches 106/106, lines 130/130; the six missed instructions are the
  generated `getEntries` of three enums.

The shared oracle has 207 cases (ENUM 49, TRANSITION 40, STANDING 14,
START 57, FINISH 47), SHA256
`068f9dbb8a7a994fdb1c424365d2c45b06655ec01a84b29b874253ea86c6d295`. Both
consumers assert the hash, counts, unique IDs, allowed argument names, all
35 state/command pairs, that each of the seven error codes has a vector,
that an `INCOMPLETE` result carries only its ordered `missing` list, and
that a finish never changes its input trip.

Named negatives mapped to tests:

- Start without an eligible context: `start-omit-asset`, `-driver`,
  `-departure`, `-declaration`, `start-omit-whole-context`,
  `start-omit-driver-and-declaration`, `start-missing-before-zone`,
  `start-departure-absent-*` (invalid reasons), `start-value-kind-mismatch`,
  `start-value-at-modulus`, `start-omit-zone`, `start-invalid-*-zone`; Go
  `TestZeroTripAndForeignFinish`, Kotlin `absent context is listed once in
  order`.
- Invalid transition: the 35 `transition-*` pairs (for example
  `transition-in-progress-start`, `transition-paused-finish`,
  `transition-planned-start`), `transition-invalid-*`; Go
  `TestClosedVocabulariesHaveNoDefaults`, Kotlin `the transition table is
  total and only finishes an in-progress trip`.
- Pause, resume and cancel refused: `transition-in-progress-pause`,
  `-resume`, `-cancel` and the PLANNED/ASSIGNED/ACCEPTED/PAUSED rows.
- Completed mistaken for approval: `standing-completed-*` (only
  `standing-completed-approved` is approved), `enum-state-invalid-approval-word`,
  `enum-command-invalid-approve`, `enum-review-invalid-operational-completed`,
  `standing-invalid-*`, `transition-invalid-approve-command`; Go
  `TestStandingSeparatesCompletionFromApproval` and
  `TestValueIsExactlyTheWorkedExample`, Kotlin `standing separates
  completion from approval` and the VAL-01 test.
- Foreign organization asset, driver, reading, author or scope:
  `start-foreign-asset`, `start-foreign-driver`, `start-foreign-reading`,
  `start-reading-of-other-asset`, `start-foreign-author`,
  `start-foreign-identity`, `start-other-organization-scope`,
  `start-malformed-scope`, `finish-foreign-reading`,
  `finish-reading-of-other-asset`, `finish-foreign-scope`,
  `finish-malformed-scope`, `finish-foreign-author`; another member
  finishing: `finish-other-member` and the 32-goroutine Go race test.
- Finish before the start instant: `finish-before-start`,
  `finish-before-start-nanosecond`,
  `finish-before-start-although-local-later` (local clock face later,
  instant earlier); accepted boundaries `finish-equal-instant`,
  `finish-after-start-although-local-earlier`.
- Terminal state reused (a new attempt is a new trip): `finish-terminal-reused`,
  `transition-completed-*`, `transition-cancelled-*`,
  `start-second-trip-new-identity`.
- Never inventing distance: `finish-c1-example` (290 000 m),
  `finish-*-absent` (READING_ABSENT), `finish-decreasing-kept-not-measured`,
  `finish-rollover-not-computed`, `finish-different-*` and every START row
  (NOT_FINISHED).
- Transitions preserve organization, asset and driver:
  `finish-preserves-asset-and-driver`, `finish-preserves-identity`,
  `finish-keeps-unit-attribution`, `finish-after-midnight-keeps-start-day`,
  the harness input-unchanged assertion, Go
  `TestInputsAreCopiedAndResultsAreValues` and
  `TestConcurrentFinishesShareOneImmutableTrip`.
- Redaction: Go `TestPrivateValuesAreRedacted` (fmt `%v %+v %#v %s %q`,
  JSON, slog JSON handler), Kotlin `private values print redacted`.

## Validation

All gates pass. Commands (Go from `backend`, toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f4-go-build`; Gradle from the worktree root with
JDK 17 under `flock /data/dev/worktrees/.f4-gradle.lock`):

```sh
gofmt -l ./internal/modules/trip                                   # empty
go build ./... && go vet ./... && go vet -tags=integration ./...   # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/trip/...                  # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/trip/... # clean
go test -race -count=1 -json -covermode=atomic -coverprofile=/data/dev/worktrees/f4-logs/trip001-go-green-coverage.out ./internal/modules/trip/domain
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...                   # exit 0: 215 packages ok, 78 without tests, 0 failures
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline --no-daemon --no-parallel --max-workers=2 --console=plain -PcomapsSdkDir=/data/dev/worktrees/enterprise-f3/build/comaps/7ad828883c4aca8a
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
```

Logs: `trip001-go-static.log` (build, vet, staticcheck),
`trip001-go-untagged-all.log`. No SQL, migration, route, authorization or
sync intake changed, so no PostGIS, integration or `organization/adapters`
suite applies; `sqlc` is not affected. The domain module has no
ktlint/detekt task. The secret grep hits are benign: the word "token" in
enum-token prose and a test loop variable named `secret` that asserts
redaction; `git diff --cached --binary | gitleaks stdin --redact` finds no
leaks. Data is synthetic.

## Retained obligations

- EMP-TRIP-002: persistence with the one-active-trip-per-asset
  serialization (VAL-07 `ASSET_BUSY` review), `trip.start`/`trip.finish`
  intake handlers, server `received_at`, the unit and cost-centre snapshot
  at start (BR-AUTH-005), server-resolved reporting zones, recording the
  departure and final readings through the fleet port with origins
  TRIP_START/TRIP_END, current `trip.trip.start` authority and assignment
  (SELF means the author's own employee record is the driver), asset
  availability, `TRIP_ALREADY_CLOSED` intake for a finish on a closed trip,
  final-reading plausibility, replay/idempotency and real PostGIS proof.
- EMP-TRIP-003: typed logbook events with stable identity and receipt.
- EMP-TRIP-004: embed the APPR-001 review lifecycle, bind `ReviewState` to
  it and decide which review states may accompany a non-completed trip.
- EMP-TRIP-005: the Android offline start/finish through the durable queue
  using this Kotlin mirror; delivery state stays the OFFL-004 presentation.
- EMP-TRIP-011 replaces the `UNSUPPORTED_COMMAND` refusals of PAUSE, RESUME
  and CANCEL (and PAUSED) with its transitions in a new oracle version;
  EMP-TRIP-008/009/021 own PLANNED, ASSIGNED and ACCEPTED; EMP-TRIP-012/014
  own handover and supervisor closure (`trip.trip.close_for_other`);
  EMP-TRIP-013 vehicle change; EMP-TRIP-015 amendments after closure;
  EMP-TRIP-017 midnight and zone crossing (next task in this worktree).
- METER owners: rollover, reset and multi-segment usage arithmetic remain
  theirs; the trip reports such cases as NOT_MEASURED and invents nothing.
- No route, SQL, migration, OpenAPI, app/panel screen, DI, job or producer
  is added; no persistence, API, app, C1, M2, M4 or F4 acceptance is claimed.
