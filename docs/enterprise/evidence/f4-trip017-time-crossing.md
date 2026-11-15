# EMP-TRIP-017 — Midnight and time-zone crossing rules

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Second bounded F4 pure-domain task in worktree branch
`codex/enterprise-f4-trip001`, on top of EMP-TRIP-001 (commit `68dc0085`,
[evidence](f4-trip001-trip-domain.md)). It extends the pure Go
`backend/internal/modules/trip/domain` package and its Kotlin mirror
`com.anpfuel.domain.enterprise.trip`, and adds a second shared oracle
`contracts/testdata/enterprise/trip/trip-time-v1.psv`; the frozen
`trip-v1.psv` and its hash are unchanged and still pass. No migration, SQL,
route, OpenAPI, Android app/data module, DI, persistence, job, producer or
dependency is added. Synthetic data only.

Rules read: the EMP-TRIP-017 task block ([EP06](../backlog/ep06-trips.md));
REQ-07.6 ([traceability](../17-traceability.md)); BR-TIME-001, BR-TIME-002,
BR-TIME-004, BR-TIME-005 and BR-TIME-006 ([shared rules](../04-business-rules.md));
BR-TRIP-004 and §§8, 10 of the [trip design](../modules/trips.md) ("crossing
midnight does not split the trip's identity or fabricate duration";
"reporting days use the unit context stored with the fact"); EDR-028 (no
working hours); B-BR-DRV-07 and the driver-shift boot identity profile
(`backend/internal/modules/driver/domain/shift.go`); the operation-time
kernel and its evidence ([EMP-PLAT-015](f1-plat015-operation-time.md)) and
the receipt-free `AttributeReportingDay` extracted by EMP-EXP-001.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-276..279, BUC-EMP-139..141, EDR-182..183. All
are used; none remains reserved.

Rules B-BR-EMP-276, B-BR-EMP-277, B-BR-EMP-278, B-BR-EMP-279, BUC-EMP-139,
BUC-EMP-140 and BUC-EMP-141 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-017 — Midnight and time-zone crossing rules (2026-11-15)", written
before any behaviour.

Decisions EDR-182 (the start attribution is kept for the trip's whole life;
FINISH takes no zone; informational end day in the stored zone; separate
entry-zone and offset flags; an unattributable end day is `INVALID`) and
EDR-183 (elapsed time only from instants, an optional per-event run clock
with the driver-shift boot profile, an explicit caller tolerance without a
domain default, statuses `CONFIRMED`/`DECLARED`/`GAP`/`NOT_FINISHED`, the
witness kept in the trip package) are recorded in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

## Shape frozen before behaviour

Go additions: `RunClock` (`NewRunClock`, `BootID`, `ElapsedMillis`),
optional `StartInput.Clock` and `FinishInput.Clock`, `Event.Clock`,
`Crossing` (`Trip.Crossing`, `Finished`, `Day`, `EndDay`,
`CrossesMidnight`, `EntryZoneChanged`, `OffsetChanged`), `Elapsed`
(`Trip.Elapsed(toleranceMillis)`, `Status`, `Duration`, `Gap`),
`ElapsedStatus` and `GapReason`. Kotlin additions: `TripRunClock`, optional
`clock` on `TripStartInput`/`TripFinishInput`, `TripEvent.clock`,
`TripCrossing` with the `Trip.crossing` extension, `TripElapsed`,
`TripElapsedStatus`, `TripGapReason` and `Trip.elapsed(toleranceMillis)`.
`RunClock`, `Elapsed` and their Kotlin counterparts print redacted.

Reuse: the end day calls the accepted `orgtime.AttributeReportingDay` /
`EnterpriseTime.attributeReportingDay` with the stored reporting zone; no
kernel changed. The exact instant difference avoids Go `time.Duration`
(saturating after about 292 years) and uses `java.time.Duration` seconds and
nanoseconds in Kotlin; the tolerance comparison uses `math/big` /
`BigInteger`. The Kotlin vector fixture builders moved unchanged into a
shared test object `TripFixture` used by both corpora.

## RED

Both languages first compiled against exported stubs (Go zero values and
`errNotImplemented`, Kotlin `TODO()`), with the frozen oracle and tests.

- Go (`go test -race -count=1 -json`), log
  `/data/dev/worktrees/f4-logs/trip017-go-red.jsonl`: 14 top-level tests;
  the 4 new ones fail (3 unit tests and the time-vector suite) and the 10
  EMP-TRIP-001 tests pass with all 207 trip-v1 subtests; 65 of 66
  time-vector subtests fail (package 0.363 s, zero skips). A first run
  stopped in the loader because its run-clock pairing check also applied to
  CLOCK rows (which use `elapsed`, not `mono`); the check now applies to
  ELAPSED only. No expectation changed.
- Kotlin (`:domain:test --tests 'com.anpfuel.domain.enterprise.trip.*'`,
  flock), log `/data/dev/worktrees/f4-logs/trip017-kotlin-red.log`, XML in
  `trip017-kotlin-red-xml/`: time unit 3/3 failing (0.055 s), time vectors
  65/66 failing (1.31 s), the EMP-TRIP-001 suites 217/217 passing after the
  fixture move; Gradle 50 s.
- The one time row that passes against the stubs in both languages,
  `crossing-finish-before-start-across-zones`, is the EMP-TRIP-001
  `FINISH_BEFORE_START` rule (B-BR-EMP-272), already GREEN; it is kept as the
  cross-zone regression guard that carries that error code in this corpus,
  not claimed as new RED.

## GREEN and REFACTOR

- Go, log `/data/dev/worktrees/f4-logs/trip017-go-green.jsonl` and coverage
  profile `/data/dev/worktrees/f4-logs/trip017-go-green-coverage.out`: the
  first implementation passed 14/14 top-level, 66/66 time and 207/207 trip
  subtests (race, atomic coverage, 1.278 s), zero failures and skips;
  statement coverage 100.0%. staticcheck (plain and `-tags=integration`) is
  clean.
- Kotlin targeted, log `/data/dev/worktrees/f4-logs/trip017-kotlin-green.log`:
  286 tests (trip vectors 208, trip unit 9, time vectors 66, time unit 3)
  pass on the first implementation (Gradle 54 s); no compiler warning in the
  trip sources.
- REFACTOR after a JaCoCo review: the `TripEvent` constructor lost an
  unused default argument and `Trip` its `endDay` default (every caller passes
  them), the crossing view uses one null check instead of a safe call plus
  elvis, and the run-clock equality test also compares a different boot
  identity. Everything was rerun green.
- Kotlin final whole enterprise suite (`--tests
  'com.anpfuel.domain.enterprise.*'`), log
  `/data/dev/worktrees/f4-logs/trip017-kotlin-enterprise-final.log`, XML in
  `/data/dev/worktrees/f4-logs/trip017-kotlin-final-xml/`: 42 suites, 2571
  tests, 0 failures, 0 errors, 0 skipped (Gradle 49 s), including the trip
  package 286. JaCoCo (`/data/dev/worktrees/f4-logs/trip017-kotlin-jacoco.xml`)
  for `com.anpfuel.domain.enterprise.trip`: instructions 1893/1903 (99.47%),
  branches 138/138, lines 173/173; the ten missed instructions are the
  generated `getEntries` of five enums.

The shared time oracle has 66 cases (CLOCK 12, CROSSING 17, ELAPSED 37),
SHA256 `c5054633152e33112d7345b5302bd264ee6a5235f399617397f3971ae45c6a22`.
Both consumers assert the hash, counts, unique IDs, allowed argument names,
paired run-clock arguments, that each of its two error codes (`INVALID`,
`FINISH_BEFORE_START`) has a vector, and that the crossing day always
equals the stored start reporting day. Expected values were written by hand:
for example 2026-03-08 and 2026-11-01 are the 2026 United States
daylight-saving Sundays, and 0001-01-01T00:00:00Z to
9999-12-31T23:59:59.999999999Z is 3 652 059 days minus one nanosecond, that
is 315 537 897 599 s and 999 999 999 ns.

Named negatives mapped to tests:

- A local clock change invents duration: `elapsed-dst-spring-forward`
  (local faces 01:30 → 03:30, 3600 s), `elapsed-dst-fall-back` (equal local
  faces, 3600 s), `elapsed-zone-crossing-forward` and `-back`,
  `elapsed-dst-local-faces-not-duration`, `elapsed-wall-clock-moved-forward`
  and `-back`, `elapsed-gap-*`, `elapsed-monotonic-backwards`,
  `elapsed-reboot*`, `elapsed-tolerance-zero-one-nanosecond`,
  `elapsed-largest-*`; Go `TestElapsedNeverInventsDuration`, Kotlin
  `elapsed time is never invented`.
- Midnight splits one trip: `crossing-midnight-one-trip`,
  `crossing-midnight-by-one-nanosecond`, `crossing-several-days-one-trip`,
  `crossing-trip-belongs-to-start-day`, `crossing-ends-last-nanosecond-of-day`;
  Go `TestClocksAreCopiedAndEventsKeepTheirContext`, Kotlin `one trip
  crosses midnight and zones without changing its basis`.
- Reporting basis lost: `crossing-reporting-basis-kept-after-travel`,
  `crossing-unit-zone-decides-end-day`,
  `crossing-device-midnight-is-not-reporting-midnight`; FINISH has no zone
  input in either language; the trip-v1 rows
  `finish-keeps-unit-attribution` and `finish-after-midnight-keeps-start-day`
  still pass.
- Every event keeps its source zone and instant:
  `crossing-other-zone-same-offset`, `crossing-offset-only`,
  `crossing-dst-*`, `crossing-equal-instant-other-zone`, the clock outputs of
  every ELAPSED row and the copy tests in both languages.
- Unattributable end day and malformed witnesses:
  `crossing-finish-day-out-of-range` (Pacific/Kiritimati, local year 10000),
  `clock-invalid-*`, `elapsed-invalid-*`, `elapsed-negative-tolerance*`.

## Validation

All gates pass. Commands as in the
[EMP-TRIP-001 evidence](f4-trip001-trip-domain.md#validation), now with logs
`trip017-go-static.log` (gofmt empty, `go build ./...`, `go vet ./...`,
`go vet -tags=integration ./...`, staticcheck plain and integration on
`./internal/modules/trip/...`: clean) and `trip017-go-untagged-all.log`
(`GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`: exit 0, 215 packages ok, 78 without tests, 0 failures).
`python3 docs/enterprise/tools/check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0.
`git diff --cached --check` is clean; the staged secret grep only matches
prose and redaction-test identifiers and `gitleaks stdin --redact` finds no
leaks. No SQL, migration, route, authorization or sync intake changed, so no
PostGIS, integration or `organization/adapters` suite applies.

## Retained obligations

- EMP-TRIP-005: the Android app supplies the run clock (boot identity and
  elapsed-realtime milliseconds) and persists it with each queued event;
  process death, reboot and missing checkpoints during an active trip
  follow BR-TIME-005 with this gap vocabulary.
- EMP-TRIP-002/003: persist both events' instant, entry zone, offset and run
  clock and the start reporting snapshot; the server keeps `received_at`
  separately (BR-TIME-002) and applies BR-TIME-006 skew flags at intake.
- The organization policy owner chooses the confirmation tolerance (no
  domain default exists); EMP-TRIP-004/006 present `DECLARED`, `CONFIRMED`
  and `GAP` honestly in review and the panel.
- EMP-TRIP-016: period locks use the start reporting day; the end day never
  decides period membership.
- Logbook events between start and finish (EMP-TRIP-003), handover
  (EMP-TRIP-012) and pause/resume (EMP-TRIP-011) reuse the same instant-only
  rule; paid hours and time-clock output stay out of scope (EDR-028,
  REQ-07.7).
- No route, SQL, migration, OpenAPI, app/panel screen, DI, job or producer
  is added; no persistence, API, app, M4 or F4 acceptance is claimed.
