# EMP-GPS-001 — Tracking session domain: trip-bound lifecycle, permission and quality states

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 pure-domain task in worktree branch
`codex/enterprise-f4-gps001` (based on the F4 head `8f7e2f94`); the
coordinator integrates it into `codex/enterprise-f4` and owns PROGRESS, the
F4 execution ledger, pushes and the PR. The owner declared EMP-GOV-005
complete and asked for tracking to be implemented for real (EDR-271);
real-device acceptance stays OWED and the physical phone was not used. This
task adds a pure Go package `backend/internal/modules/tracking/domain`, its
Kotlin mirror `com.anpfuel.domain.enterprise.tracking` (the Android tracking
service of EMP-GPS-002 will run the session offline) and one shared oracle
`contracts/testdata/enterprise/tracking/session-v1.psv`. No migration, SQL,
route, OpenAPI, Android app/data module, DI, persistence, job, producer,
location provider or dependency is added. Synthetic data only; the domain
holds no coordinates.

Rules read: the EMP-GPS-001 task block and the EMP-GPS-002..015 blocks
([EP07](../backlog/ep07-gps-navigation.md)); the whole
[tracking specification](../modules/gps-navigation.md) (BR-GPS-001..040,
UC-GPS-01..11, §6.1 session transitions, §6.2 track quality, §8.2
interruption matrix, §10.2 sampling, §10.4 coverage and the 950-permille
example, §10.10 codes and gap reasons); the
[state catalog](../08-state-model.md) §§1, 3, 4 (Tracking session
`NOT_STARTED, RECORDING, SUSPENDED, INTERRUPTED, ENDED`; Track quality
`PRECISE, APPROXIMATE, INSUFFICIENT`; closed lists, terminal states,
transitions as commands); the location rules of
[privacy](../11-privacy-security.md) (BR-PRIV-006/008/015, retention row,
LV-01/03/06); `gps.location.read_live` and `gps.track.read` with the
OWN_TRIPS_ONLY / RECORDED_TRACKS_ONLY footnotes
([permissions](../07-permissions.md) §7.3); BR-OFFL-003 and the tracking row
of the offline matrix ([offline sync](../10-offline-sync.md)); VAL-27/VAL-28
and the device owed-evidence rows ([validation](../15-validation.md));
REQ-08.4 ([traceability](../17-traceability.md)); EDR-015..018, EDR-026,
EDR-028, EDR-178..183, EDR-271/272 and CX-06
([decisions](../16-decisions.md)); the trip domain evidence
[EMP-TRIP-001](f4-trip001-trip-domain.md) and
[EMP-TRIP-017](f4-trip017-time-crossing.md) (no EMP-TRIP-011 evidence exists
on this base: pause, resume and cancel are still refused by the trip domain,
so the session consumes PAUSED and CANCELLED only as supplied trip facts). The
module name `tracking` and the Android package follow
[09-architecture](../09-architecture.md) §2. Precedents copied: EMP-TRIP-001
and EMP-TRIP-017 (structure, oracle grammar, hash assertions, redaction).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-421..427, BUC-EMP-214..216, EDR-273..276. All
are used; none remains reserved.

Rules B-BR-EMP-421 (trip-bound opening), B-BR-EMP-422 (explicit start and
resume with the permission stated), B-BR-EMP-423 (pause suspends; typed
interruptions and reduced precision), B-BR-EMP-424 (end is terminal),
B-BR-EMP-425 (the trip never depends on the session; collection),
B-BR-EMP-426 (recorded history, periods, immutability, redaction),
B-BR-EMP-427 (track quality classification) and the use cases BUC-EMP-214,
BUC-EMP-215 and BUC-EMP-216 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-001 — Tracking session domain: trip-bound lifecycle, permission and
quality states (2026-11-15)", written before any behaviour.

Decisions, recorded in [16-decisions](../16-decisions.md) as the recommended
routine choices, taken unattended:

- EDR-273: placement (Go `tracking/domain`, Kotlin `enterprise.tracking`, one
  session-v1 oracle); reuse of the trip domain's `TripRef`, `MembershipRef`
  and operational state; the bound trip's current facts supplied with every
  command instead of a stored copy; `tracking` imports `trip`, so Go's
  acyclic imports guarantee that no trip transition can read the session;
  device, account and origin binding left to EMP-GPS-002.
- EDR-274: the closed origins, causes, actor kinds and refusal codes. The
  causes are the §10.10 session reasons plus the §6.1 situations named
  `TRIP_PAUSED`, `TRIP_COMPLETED`, `TRIP_CANCELLED`, `MEMBERSHIP_ENDED`,
  `DEVICE_REVOKED`; `NOTIFICATION_BLOCKED` is also an interruption cause
  (BR-GPS-004/015). `SIGNAL_LOST`, `PROVIDER_DISABLED` and `MOCK_LOCATION`
  are fix-level gap reasons inside a recording period and stay with
  EMP-GPS-003/005 (they are `INVALID` as session causes).
- EDR-275: a start or resume refused for permission `NONE` or a hidden
  notification keeps the state and records the attempt as the visible cause
  (no transition outside §6.1); precision is decided at each start or resume
  and only reduced in between; policy `OFF` also stands for an inactive
  capability or an organization whose employee notice is not in force
  (EDR-271), checked only at opening; the notice at each start stays with
  EMP-GPS-002/015.
- EDR-276: the BR-GPS-026 classification as one pure function of counted
  inputs with an overflow bound of 10¹² expected seconds; the per-fix filter
  (BR-GPS-017), gaps, distance and divergence stay with EMP-GPS-003/005/006.

## Shape frozen before behaviour

Go package `backend/internal/modules/tracking/domain`: errors `ErrInvalid`,
`ErrScope`, `ErrForbidden`, `ErrTrackingOff`, `ErrPersonalTrip`,
`ErrImplicitStart`, `ErrSessionExists`, `ErrInvalidTransition`,
`ErrTripState`, `ErrOutOfOrder`; closed `State` (`Terminal`), `Permission`,
`Policy`, `Origin`, `Cause`, `Actor`, `Quality` with `Parse*`; `SessionRef`
(`NewSessionRef`), `TripFacts`, `Device`, `OpenInput`, `DriverAction`,
`Event`, `Transition`, `Period`; `Session` with `Open`, `Start`, `Resume`,
`Suspend`, `Interrupt`, `ReducePrecision`, `End`, `Follow`, `Collecting`,
`State`, `Cause`, `Precision`, `Coarse`, `History`, `RecordingPeriods`;
`TrackEvidence`, `Classification`, `Classify`, `MaxExpectedSeconds`. Every
private value (references, inputs, sessions, history, periods) prints
redacted in fmt, slog and JSON.

Kotlin mirror `com.anpfuel.domain.enterprise.tracking`: `TrackingFailure`,
`TrackingDomainException`, `TrackingSessionState`, `LocationPermission`,
`TrackingPolicy`, `TrackingOrigin`, `TrackingCause`, `TrackingActor`,
`TrackQuality`, `TrackingSessionRef`, `TrackingTripFacts`, `TrackingDevice`,
`TrackingOpenInput`, `TrackingDriverAction`, `TrackingEvent`,
`TrackingTransition`, `TrackingPeriod`, `TrackingSession` (`open`, `start`,
`resume`, `suspend`, `interrupt`, `reducePrecision`, `end`, `follow`,
`collecting`), `TrackEvidence`, `TrackClassification`,
`TrackQualityRules.classify`; every `toString` is redacted.

Reuse: trip references and `trip.State`/`TripState` from EMP-TRIP-001; the
Kotlin scope check reuses `EnterprisePartition.requireId`. No kernel or trip
file changed. `scripts/check-manifest.txt` lists
`backend/internal/modules/tracking/domain` in `[quick-packages]`.

## RED

Both languages first compiled against exported stubs (Go zero values and
`errNotImplemented`, Kotlin `TODO()`), with the frozen oracle and tests.

- Go (`go test -race -count=1 -json`), log
  `/data/dev/worktrees/f4-logs/gps001-go-red.jsonl`: 7 top-level tests, 5
  failing, and 359/359 shared-vector subtests failing (package 0.22 s): 220
  because the fixture session could not open and 139 with the unmapped
  `not implemented` error. The two passing top-level tests are the fixture
  grammar reader and `TestTripDoesNotDependOnTracking`, a structural guard
  that is true before any behaviour (the trip packages never imported
  tracking); neither is claimed as RED. Zero skips.
- Kotlin (`:domain:test --tests 'com.anpfuel.domain.enterprise.tracking.*'`,
  flock), log `/data/dev/worktrees/f4-logs/gps001-kotlin-red.log`, XML in
  `gps001-kotlin-red-xml/`: 364 tests, 363 failures, 0 errors, 0 skipped
  (vectors 359/359 and unit 4/5 failing on `NotImplementedError`; the one
  pass is the same structural guard `the trip never depends on tracking`;
  Gradle 42 s). The vector factory's own preconditions (hash, counts,
  arguments, error-code coverage) already passed against the frozen corpus.

## GREEN and REFACTOR

- Go, log `/data/dev/worktrees/f4-logs/gps001-go-green.jsonl`, coverage
  `gps001-go-green-coverage.out`: the first implementation passed 7/7
  top-level and 359/359 subtests (race, atomic coverage, 1.18 s), statement
  coverage 100.0%.
- Kotlin targeted, log `gps001-kotlin-green.log`: 364 tests pass (vectors
  359, unit 5), 0 failures, errors or skips (Gradle 15 s); no compiler
  warning in the tracking sources.
- REFACTOR: staticcheck found an unused fixture helper (removed); the Go
  from-state check now uses one generic `contains` shared with the trip-state
  rule; the Kotlin reference checks return the checked value instead of
  `!!` assertions. Go rerun, log `gps001-go-refactor.jsonl`, coverage
  `gps001-go-refactor-coverage.out`: 7/7, 359/359, 100.0% statements
  (1.15 s); staticcheck plain and `-tags=integration` clean.
- Kotlin final whole enterprise suite (`--tests
  'com.anpfuel.domain.enterprise.*'`), log
  `gps001-kotlin-enterprise-final.log`, XML in `gps001-kotlin-final-xml/`:
  58 suites, 3 905 tests, 0 failures, 0 errors, 0 skipped (Gradle 1 m 5 s),
  including tracking 364. JaCoCo (`gps001-kotlin-jacoco.xml`) for package
  `com.anpfuel.domain.enterprise.tracking`: lines 214/214, branches 161/164,
  instructions 2347/2379; the missed instructions are data-class getters that
  are only read through destructuring, a private enum's generated
  `getEntries` and the unreachable null-permission guard of
  `recordingPeriods`.

The shared oracle has 359 cases (ENUM 64, OPEN 55, START 53, RESUME 30,
SUSPEND 23, INTERRUPT 28, REDUCE 13, END 23, FOLLOW 22, COLLECT 22,
QUALITY 26), SHA256
`4ddac666121fe1183e067dd65589a57c3f97a82e230e3091454ce9b889f9d39e`. Both
consumers assert the hash, counts, unique IDs, allowed argument names, that
each of the ten error codes has a vector and no other appears, complete
result maps and that a command never changes its input session. Expectations
were written by hand from the rules (the authoring aid only joined history and
period strings); neither consumer computes them.

Named negatives mapped to tests:

- Personal trip collected: `open-personal-origin`, `start-personal-origin`,
  `resume-personal-origin`, `open-personal-before-policy`,
  `start-personal-before-member`; a session without an organization scope:
  `open-malformed-scope`, `start-malformed-scope`, `end-malformed-scope`,
  `collect-malformed-scope`.
- Implicit start (boot, worker, push, geofence, server, notification):
  `open-*-origin`, `start-*-origin`, `resume-{notification,boot,worker,server}-origin`,
  `open-implicit-before-member`; recording never resumes by itself:
  `follow-trip-resumed-keeps-suspended`, `collect-suspended-trip-resumed`,
  Go `TestTrackedTripJourney`, Kotlin journey test.
- Duplicate session for one trip: `open-duplicate-session`,
  `open-duplicate-after-ended`, `open-trip-state-before-duplicate`,
  `open-existing-of-other-trip`, `open-existing-of-other-organization`;
  repeated start `start-twice`; 32-goroutine Go race test.
- Missing permission hidden: `start-permission-denied-recorded`,
  `start-notification-blocked-recorded`,
  `start-approximate-notification-blocked`,
  `start-permission-checked-before-notification`,
  `resume-denied-keeps-suspended`, `resume-denied-keeps-interrupted`,
  `resume-blocked-keeps-interrupted`, `start-after-denied-attempt`,
  `end-after-denied-start`, `collect-permission-denied`,
  `collect-notification-blocked`; an absent permission is `INVALID`, never
  assumed: `start-omit-permission`, `resume-omit-permission`,
  `start-invalid-permission`, `enum-permission-invalid-*`, Kotlin `absent
  inputs are invalid and never assumed`.
- Approximate-only permission: `start-approximate-only`,
  `resume-approximate-only`, `resume-raises-precision`, `reduce-precision`,
  `reduce-after-raised-resume`, `reduce-already-approximate`,
  `collect-recording-approximate-is-coarse`,
  `collect-after-reduction-is-coarse`, `end-keeps-coarse-period`,
  `quality-worked-example-approximate`, `quality-coarse-*`.
- Session continuing after the trip ended: `start-trip-completed`,
  `start-trip-cancelled`, `start-on-session-of-ended-trip`,
  `resume-after-trip-completed`, `resume-ended-by-trip`,
  `interrupt-trip-completed`, `suspend-pause-with-trip-completed`,
  `follow-completed-*`, `follow-cancelled-*`,
  `collect-trip-completed-not-followed`, `collect-trip-cancelled-not-followed`,
  `collect-ended`, `collect-ended-membership-trip-running`; terminal ENDED:
  `start-on-ended`, `resume-ended-by-membership`, `suspend-ended`,
  `interrupt-ended`, `reduce-ended`, `end-ended`, `end-ended-by-membership`.
- Pause with the trip: `suspend-trip-paused`, `follow-paused-*`,
  `resume-while-trip-still-paused`, `collect-suspended`,
  `collect-trip-paused-not-followed`, `suspend-pause-with-trip-in-progress`.
- Interruption causes typed, never hidden: `interrupt-permission-revoked`,
  `-notification-blocked`, `-process-stopped`, `-device-restarted`,
  `-stopped-by-driver`, `-owner-changed`, `resume-after-*`,
  `interrupt-cause-*` and `suspend-cause-*`/`end-cause-*` (a cause of
  another command is `INVALID`), `enum-cause-invalid-*-fix-gap`.
- Foreign organization or trip: `open-foreign-{scope,session,trip,driver,author}`,
  `start-foreign-*`, `start-other-trip`, `start-other-trip-before-personal`,
  `resume-other-trip`, `suspend-other-trip`, `interrupt-other-trip`,
  `end-other-trip`, `follow-other-trip`, `collect-other-trip`,
  `collect-foreign-driver`, `*-foreign-scope`,
  `interrupt-stop-by-foreign-author`.
- Another member: `open-author-not-driver`, `open-for-another-drivers-trip`,
  `start-other-member`, `start-trip-handed-over`,
  `start-other-member-now-driving`, `resume-other-member`,
  `resume-trip-handed-over`, `interrupt-stop-by-other-member`,
  `interrupt-stop-member-before-transition`, `collect-trip-handed-over`.
- Tracking off: `open-policy-off`, `open-member-before-policy`,
  `open-policy-before-trip-state`.
- The trip works without GPS: `TestTripDoesNotDependOnTracking` and Kotlin
  `the trip never depends on tracking` (no trip source references
  tracking); FOLLOW never answers `INVALID_TRANSITION`
  (`follow-completed-keeps-ended`, `follow-paused-keeps-ended`,
  `follow-paused-keeps-not-started`); a session that never started still
  ends with its trip (`end-not-started-trip-completed`,
  `follow-completed-ends-not-started`).
- Out-of-order instants: `start-out-of-order`, `start-before-opening`,
  `resume-out-of-order`, `suspend-out-of-order`, `interrupt-out-of-order`,
  `reduce-out-of-order`, `end-out-of-order`, `follow-out-of-order-transition`;
  equal instants accepted (`*-equal-instant`, `start-at-opening-instant`);
  unset instants `INVALID` (`*-zero-instant`, `*-omit-at`).
- Quality: `quality-worked-example` (950, `PRECISE`), the 900/899/500/499
  boundaries, floor, no expected time, fewer than two fixes, the bound and
  the invalid inputs.
- Immutability and redaction: harness input-unchanged assertions in both
  languages; Go `TestConcurrentCommandsShareOneImmutableSession` (32
  goroutines, `-race`, branches never share history),
  `TestPrivateValuesAreRedacted` (fmt `%v %+v %#v %s %q`, JSON, slog JSON
  handler); Kotlin `private values print redacted`.

## Validation

All gates pass. Commands (Go from `backend`, toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f4-go-build`; Gradle from the worktree root with
JDK 17 under `flock /data/dev/worktrees/.f4-gradle.lock`):

```sh
gofmt -l ./internal/modules/tracking                               # empty
go build ./... && go vet ./... && go vet -tags=integration ./...   # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/...                  # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... # clean
go test -race -count=1 -json -covermode=atomic -coverprofile=/data/dev/worktrees/f4-logs/gps001-go-refactor-coverage.out ./internal/modules/tracking/domain
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...                   # exit 0: 224 packages ok, 81 without tests, 0 failures
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline --no-daemon --no-parallel --max-workers=2 --console=plain -PcomapsSdkDir=/data/dev/worktrees/enterprise-f3/build/comaps/7ad828883c4aca8a
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
```

Logs: `gps001-go-static.log` (build, vet, staticcheck, gofmt),
`gps001-go-untagged-all.log`. No SQL, migration, route, authorization or sync
intake changed, so no PostGIS, integration or `organization/adapters` suite
applies; `sqlc` is not affected. The domain module has no ktlint/detekt task.
No emulator or device was used: the task is a pure domain. The secret grep hits are benign:
the word "token" in enum-token prose, the `go/token` import and the
redaction tests' `secret` loop variables; `git diff --cached --binary |
gitleaks stdin --redact` finds no leaks. Data is synthetic.

## Retained obligations

- EMP-GPS-002: the Android foreground service of type `location` with its
  notification and the location-guard update; owner binding to account,
  origin, organization, membership and device (BR-GPS-014); the notice at
  each tracking start (BR-GPS-034, with EMP-GPS-015); one listener and safe
  stop (BR-GPS-013); translating watchdog observations and checkpoint
  recovery into these commands.
- EMP-GPS-003: sampling, durable buffer and checkpoint (BR-GPS-012/018/019/
  020), the per-fix filter of BR-GPS-017 on the device and the fix-level gap
  reasons `SIGNAL_LOST`, `PROVIDER_DISABLED`, `MOCK_LOCATION`; fixes already
  buffered stay queued after `ENDED` (BR-GPS-010 device side).
- EMP-GPS-004: persistence (one session per trip enforced by the database),
  the `gps.session.*` operations with replay and idempotency, server session
  bounds from the recording periods (BR-GPS-022), held sessions and the
  device named per segment; real PostGIS proof.
- EMP-GPS-005: deriving expected and gap seconds and accepted fixes from
  stored fixes, gaps (BR-GPS-024), GPS distance and simplified geometry, then
  calling the classification fixed here.
- EMP-GPS-006: distance basis and divergence; EMP-GPS-010: the interruption
  matrix on the API36 emulator; EMP-GPS-015: transparency screens.
- EMP-TRIP-011 (in progress in parallel): the trip domain's pause, resume
  and cancel transitions produce the PAUSED and CANCELLED facts that FOLLOW
  consumes; EMP-TRIP-012/014: session ownership after a handover or a
  supervisor closure (today a handed-over trip refuses the old owner's start
  and resume and collects nothing; rebinding is not decided here).
- BR-GPS-006: marking an `EXPECTED` trip "sem rastreamento" and resolving the
  organization's policy from its capability and its employee notice in force
  (EDR-271) belong to the application layer; BR-GPS-016 detection of a long
  stationary period belongs to the device.
- Real-device acceptance stays OWED (EDR-271); the physical phone was never
  used. No route, SQL, migration, OpenAPI, app/panel screen, DI, job or
  producer is added; no persistence, API, app, M4 or F4 acceptance is
  claimed.
