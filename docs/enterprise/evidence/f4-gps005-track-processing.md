# EMP-GPS-005 — Track processing: filtering, gaps, GPS distance and simplified geometry

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 job task in worktree branch `codex/enterprise-f4-gps005`, on
top of the tracking policy command commit `4d5159c7` (built on the F4 head
`0912b48a`) and the merge `7f63d4b0` of the F4 head `2f031276` (EMP-EXP-002,
migration 000244; neither migration of this branch redefines a usage
function). The coordinator integrates it and owns PROGRESS, the F4 ledger,
pushes and the PR. Under the owner decision EDR-271 (tracking implemented for
real; activation per organization only with its notice in force;
real-device acceptance OWED) the server now derives, for every ended
tracking session, one summary revision from its stored fixes: per-fix
filtering, gaps with reasons, coverage and quality (the EMP-GPS-001
function), GPS distance with its method and a simplified geometry with its
tolerance. Synthetic data only; no emulator, device or physical phone; no
Android code.

Rules read: the EMP-GPS-005 block and its neighbours GPS-004/006/009/011
([EP07](../backlog/ep07-gps-navigation.md)); the
[tracking specification](../modules/gps-navigation.md) (§3 gap and track
summary entities, BR-GPS-007, BR-GPS-017, BR-GPS-023..026, BR-GPS-035/036,
BR-GPS-039, UC-GPS-05/07, §6.2 track quality, §8.1 clock and concurrency,
§8.2 gap reasons, §10.2 storage defaults — tolerance 20 m and at most 1 000
points — §10.3/§10.4 and the 950-permille example, §10.10 gap reasons);
[privacy](../11-privacy-security.md) (BR-PRIV-003/008/009, raw-fix and
simplified-track retention rows); [validation](../15-validation.md) VAL-27 and
VAL-28; the [GPS-001](f4-gps001-tracking-session-domain.md) evidence
(EDR-276 classification), the [GPS-004](f4-gps004-track-intake.md) evidence
(insert-only segments with integer arrays and MultiPoint SRID 4326, counters,
boot identity stored for this task, retained list) and the
[GPS-012](f4-gps012-location-access-audit.md) evidence; the job precedents
[DOC-014](f3-doc014-upload-safety.md) and
[RPT-001](f3-rpt001-export-infrastructure.md) (lease fencing, dedupe keys,
slots), the FLEET-007 reservation outcome job (enqueue in the business
transaction, general dispatcher) and `platform/jobs` (SKIP LOCKED claims,
fenced completion); the app's `TrackRecordingPolicy` (10 m step, sphere
6 371 008.8 m).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-461..466, BUC-EMP-234..236, EDR-298..301, all
used; migration 000246 used.

Rules B-BR-EMP-461 (an ended session is processed by one fenced job),
B-BR-EMP-462 (fixes are filtered, counted and never edited), B-BR-EMP-463
(gaps are data with a reason and a detector), B-BR-EMP-464 (GPS distance only
inside runs), B-BR-EMP-465 (one insert-only summary revision records its
basis) and B-BR-EMP-466 (retries converge; a stale job never overwrites) with
BUC-EMP-234..236 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-005 — Track processing: filtering, gaps, GPS distance and simplified
geometry (2026-11-15)", written before any behaviour.

Decisions in [16-decisions](../16-decisions.md), taken unattended as the
recommended routine choices:

- EDR-298: placement (pure Go `tracking/domain`, `tracking/application`
  `Summarize`, `tracking/adapters` store and handler, general dispatcher); the
  trigger is the tracking store (an `ENDED` transition, or a segment of an
  ended session, queues the job in the intake transaction); no Kotlin mirror
  or shared vector file (server-only job).
- EDR-299: the filter (outside, accuracy, speed with the previous accepted
  precise fix as anchor, coarse never measured), the gap rules and detectors,
  floor/ceiling rounding, the trip window (finish, cancellation, closure,
  else the session's end), device-only facts kept as counters,
  `CLOCK_UNCERTAIN` counted without rewriting any instant.
- EDR-300: the anchor distance rule, floor once, absent when unmeasurable;
  Douglas–Peucker in metres with tolerance doubling up to 1 000 vertices;
  integer arrays plus a derived `MULTILINESTRING` of SRID 4326.
- EDR-301: migration 000246 and its guard; the queue-row lease fence in the
  processing transaction.

## Shape frozen before behaviour

- `tracking/domain/processing.go`: constants `MethodVersion`
  (`gps-track-v1`), `DistanceMethod` (`haversine-r6371008.8-step10-v1`),
  `GapThreshold` (180 s), `MaxPreciseAccuracyMetres` (50),
  `MaxSpeedMetresPerSecond` (70), `MinStepMetres` (10), `EarthRadiusMetres`
  (6 371 008.8), `SimplifyToleranceMetres` (20), `MaxGeometryPoints` (1 000),
  `MaxToleranceMetres`, `MaxProcessedFixes`; `Detector`, `GapReason`
  (`GapSignalLost`, `GapReasonOf`, `ParseGapReason`), `Interval`, `Span`,
  `TrackFix`, `TrackInput`, `Gap`, `Point`, `TrackResult`; `Metres`,
  `ProcessTrack`, `Simplify`. Every value holding a position prints redacted.
- `tracking/application/processing.go`: `ProcessKind`
  (`org-tracking-process`), `ProcessAttempts` (5), `ClockTolerance` (2 s),
  `ErrStaleLease`, `ProcessOutcome` (`WRITTEN`, `CURRENT`, `NOT_ENDED`,
  `MISSING`), `Lease`, `Watermark` (`Newer`), `ProcessingSegment`, `Summary`,
  `Summarize`, port `Processor`.
- `tracking/adapters/processing.go`: `ProcessPayload`, `ProcessDedupe`,
  `ParseProcess`, `requestProcessing` (called by `Store.RecordSessionTx` when
  the stored transitions end with `ENDED` and by `Store.InsertSegmentTx`),
  `ProcessingStore` (`Process`), `ProcessHandler` (`Kind`, `Version`,
  `Handle`).
- `platform/enterprisesuite/tracking_jobs.go` `TrackingHandlers`;
  `cmd/worker/tracking.go` `trackingWorkerHandlers` composed on the general
  dispatcher in `cmd/worker/main.go`.
- `db/migrations/000246_organization_tracking_summaries.sql` (as committed).
- No route, OpenAPI, inventory, matrix, auditor-walk, sqlc, Kotlin or
  Android change; no dependency. The tracking store gains the two queue calls
  and nothing else.

## RED

The tests were written against compiled stubs (`Metres` 0, `ProcessTrack`
not implemented, `Simplify` empty, `Summarize` unavailable, `Newer` false,
no queueing, `Process` unavailable, `Handle` unavailable) and the migration
as written.

- Unit (`go test -race ./internal/modules/tracking/... ./internal/platform/enterprisesuite/`),
  log `/data/dev/worktrees/f4-logs/gps005-unit-red.jsonl`: 17 new top-level
  tests, 16 failing (12 domain, 3 application, 1 adapter). The passing one is
  `TestTrackingHandlersComposition`, which checks only the composition that is
  part of the shape; it is not claimed as RED. Every earlier tracking test
  still passed.
- PostGIS (`-tags=integration -race -run TestTrackProcessing`), logs
  `gps005-db-red.jsonl`/`.stderr`: 4/4 failing (no job queued at the end;
  the open session unavailable; no lock wait; no job before the upgrade).

## GREEN and REFACTOR

- Unit, log `gps005-unit-green.jsonl`, coverage
  `gps005-unit-green-coverage.out`: every package passes (42 top-level tests
  in the tracking packages and the composition test).
- PostGIS, log `gps005-db-green.jsonl`: 4/4 under `-race`. The first run
  (`gps005-db-green1.jsonl`) failed only in the test: an `INSERT … SELECT`
  under another scope selects no row and so raised nothing; the scope refusal
  now inserts explicit values under the foreign scope (`42501`).
- Focused tracking PostGIS suite on the merged tree
  (`-run 'TestTracking|TestTrackRead|TestTrackProcessing'`), log
  `gps005-db-focused.jsonl`: 18 top-level, 26 subtests, 0 failures — the
  EMP-GPS-004 intake tests, which now also queue a job when a session ends,
  are unchanged and green.
- Drafting the domain tests against the implementation corrected two test
  oracles before the RED run, recorded here: §10.3 gives 138.9 m for
  12 670 × 10⁻⁷ degrees at 10° south from 109 629 m per degree (an
  ellipsoid); on the §10.4 sphere it is 138.74 m, which the test pins; and a
  30 m bump drawn with 11 m vertex spacing keeps its neighbours too (they are
  more than 20 m from the chord), so the bump test uses 1.1 m spacing. No
  expectation was weakened to pass.

Named negatives mapped to tests (PostGIS through the real signed intake, the
real queue and the real dispatcher unless noted):

- Outlier accepted: `TestTrackProcessingDerivesTheSummaryOnceTheSessionEnds`
  (a fix 5.5 km off the line 10 s after the previous one is counted `SPEED`
  and an accuracy-60 m fix `ACCURACY`; neither is in the distance, which equals
  an independent haversine sum of the clean runs, nor in the geometry); unit
  `TestOutliersAndInaccurateFixesAreRejectedAndCounted` (51 m refused, 50 m
  kept, 69.997 m/s kept, 71.2 m/s refused, the anchor stays on the last
  accepted fix).
- Gap bridged silently: same PostGIS test (a 200 s hole is one `SIGNAL_LOST`
  gap from the last fix to the next, the geometry has two parts and the
  distance is below the bridged sum); unit `TestGapsAreNeverBridged`
  (181 s is a gap, exactly 180 s is not, leading and trailing gaps),
  `TestNotRecordingTimeIsAGapWithItsReason` (`NOT_STARTED` by the server,
  `STOPPED_BY_DRIVER` by the device, an early `MEMBERSHIP_ENDED`, paused time
  not expected), `TestWorkedCoverageExampleYields950` (§10.4: 240 s + 780 s
  of 20 400 s → 950 permille, `PRECISE`) and `TestVal28InterruptionsBecomeGaps`
  (VAL-28 server side: 900 s and 360 s gaps, 825 permille).
- Stale job overwrites: `TestTrackProcessingRefusesStaleForeignAndOpenInputs`
  (once a newer input exists, a summary of the older input — what a stale job
  would write — is refused, as are a skipped or repeated revision and a
  changed transition count; the stored revision is unchanged), and the lease
  fence in the first test (after a redelivery the superseded holder's run is
  `ErrStaleLease`, writes nothing, and its acknowledgement is stale).
- Foreign segment: refusal test (a summary whose fix total or intake counters
  are not the session's own segments → `23514`; another scope → `42501`; a
  foreign organization's job naming the session completes as `MISSING` with
  nothing written; forced RLS hides every summary from another scope).
- Job run twice: first test (a second delivery of the same input and the
  redelivered job converge as `CURRENT`; replaying the end operation queues no
  second job); refusal test (the queued job after a candidate of its input
  converges).
- Concurrent runs: `TestTrackProcessingConcurrentRunsConvergeInBothOrders`
  (two runs queued behind a held trip row lock, both orders: the first writes
  the revision, the other is `CURRENT`; one revision per input).
- Open session: refusal test (the job completes `NOT_ENDED`; the database
  refuses a summary of a session without `ENDED`).
- Insert-only: refusal test (`UPDATE`/`DELETE` by the runtime login and
  `TRUNCATE` → `55000`).
- Structure: refusal test (overlapping gaps, a device-detected
  `SIGNAL_LOST`, an unknown reason, gap seconds not the ceiling of the gaps,
  a line part of one vertex, an overclaimed quality, a distance without its
  method → `23514`; a structurally valid candidate is accepted as the positive
  control).
- Populated upgrade: `TestTrackProcessingPopulatedUpgradeFailureRecoveryAndPreviousImage`
  (an ended session with its segments and its queued job before 000246,
  ordinary non-superuser migrator, injected `1/0` leaves no table, function or
  row change, recovery applies once, replay no-op, previous image compatible,
  the table owned by the migrator with forced RLS, the pre-upgrade job then
  writes revision 1 with the independent distance, the remaining chain
  (000247) applies).
- No coordinate leaks: the job payload holds only version, organization and
  session (first test); unit `TestProcessingValuesAreRedacted` (fmt, JSON and
  slog of inputs, results and fixes) and
  `TestProcessJobEnvelopeKeyAndHandler` (a provider error reaching the queue is
  the fixed `unavailable` text).
- Inconsistent rows: unit `TestProcessTrackRefusesInconsistentInput` and
  `TestSummarizeRefusesInconsistentRows` (not ended, another trip, a trip
  timeline going backwards, a repeated segment sequence, a repeated instant).

## Validation

All gates ran on the merged tree (`7f63d4b0` + this commit), commands from
`backend/` with toolchain go1.27.4 (`gps005-env.sh`),
`GOCACHE=/data/dev/cache/f4-go-build`, the shared PostGIS (isolated child
databases) and a throwaway MinIO project `f4-gps005-s3` (`down -v`
afterwards):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...     # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/... ./internal/platform/enterprisesuite/... \
  ./internal/platform/orgapi/... ./cmd/worker/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... \
  ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite/... ./cmd/worker/...   # clean
/home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff            # no diff (no query or sqlc.yaml change)
go test -race -count=1 ./internal/modules/tracking/... ./internal/platform/enterprisesuite/
go test -tags=integration -race -count=1 -p 2 -json -timeout=120m ./internal/modules/organization/adapters \
  ./internal/modules/tracking/... ./internal/modules/trip/... ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/... ./cmd/worker/...
go test -count=1 -timeout=45m ./...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
git diff --cached --binary | gitleaks stdin --redact
```

- Static: `gps005-static.log` empty (staticcheck plain and integration); vet
  both tags clean; gofmt empty; `sqlc vet`/`sqlc diff` clean.
- Race/integration union, logs `gps005-db-union.jsonl`/`.stderr`/`.exit`
  (04:41–05:39Z): 33 packages, 1 173 top-level and 8 319 named subtests. The
  only failures are the 5 known `cmd/worker` tests that need the pinned Rust
  preparer CLIs this machine lacks (`TestEnforcementWorkerNative…`,
  `TestNativeFineWorker…`, `TestFineWorkerCompanyRetirement…`,
  `TestNativeWorkerRustDirectoryPublicationAndRetirement`,
  `TestNativeWorkerDirectoryRevalidation`); the organization adapters
  (including the AUTH-011 walk at 155 / 32 / 123, the 300-cell matrix and the
  MinIO document/export suites), tracking, trip, orgsync, platform and cmd/api
  packages pass. This union also covers the tracking policy command commit on
  the merged tree; the `ContactChange` race seen once in that commit's earlier
  union passed here.
- Untagged `go test -count=1 -timeout=45m ./...`, log
  `gps005-untagged-all.log`, exit 0 (230 packages pass, 86 without tests). It
  covers both commits of this branch.
- OpenAPI lint (unchanged by this task), `gps005-vacuum.log`: 0 errors,
  1 109 warnings, 681 informs — the same as the F4 head `2f031276`.
- No Kotlin, Android or emulator run: the job is server-only and no shared
  vector file exists (EDR-298).

## Retained obligations

- A trip finished, cancelled or closed without the device's `gps.session.end`
  reaching the server queues no processing (the trip packages never import
  tracking, EDR-273): a consumer of the trip completion events or a
  session-closing owner (EMP-TRIP-012/014 follow-up, GPS-002) must end the
  session or queue the job; a trip finish stored after the session's end does
  not reprocess (the window then ends at the session's end).
- EMP-GPS-006: the four distances, the distance basis (`por GPS` only with
  `PRECISE`) and the divergence signal read these summaries; no read route,
  OpenAPI view or company read of summaries exists yet (EMP-GPS-009 replay and
  SCR-GPS-07).
- EMP-GPS-011: raw-fix retention deletes segments, which lowers the input
  counts; a purge must not queue reprocessing, and the summary and simplified
  geometry follow the trip (BR-GPS-035). BR-GPS-036 trimming recomputes as a
  new revision after a corrected end.
- Held and late tracking (OFFL-009) and handover (TRIP-012): segments stored
  later through those owners must queue processing the same way.
- Monotonic instant correction stays a counter (`CLOCK_UNCERTAIN`): no trusted
  anchor exists to rewrite device instants, and fixes are never edited.
- Production grants for `org_tracking_summaries` (runtime `SELECT, INSERT`)
  and the worker's `UPDATE` on `job_queue` inside the business transaction are
  not seeded, as for every enterprise table.
- The F4 head's `16-decisions.md` carries EDR-262..264 twice (from an earlier
  integration, not this branch); left unchanged for the coordinator.
- Real-device acceptance stays OWED (EDR-271). No M4 or F4 acceptance is
  claimed.
