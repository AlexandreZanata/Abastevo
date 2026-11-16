# EMP-GPS-006 — Distance sources on a trip and the divergence signal

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task (module GPS, type DOM, size S, validation V-DOM and
V-MONEY) in worktree branch `codex/enterprise-f4-gps006`, created from the F4
integration head `7eae7bb1` (EMP-GPS-005 track processing, EMP-METER-005
segments and EMP-METER-006 `MeterStore.UsageBetween`). The coordinator
integrates it and owns PROGRESS, the F4 ledger, pushes and the PR. Under
EDR-271 (tracking implemented for real, real-device acceptance OWED) a trip's
odometer, GPS, planned and estimated distances are now evaluated apart with a
declared basis and a divergence signal that is evidence for review; a server
port composes them from the stored track summary and the meter usage. No
route, no migration, no Kotlin or Android change. Synthetic data only; no
emulator, device or physical phone.

Rules read: the EMP-GPS-006 block and its neighbours GPS-005/007/009
([EP07](../backlog/ep07-gps-navigation.md)); the
[tracking specification](../modules/gps-navigation.md) (§1, §3 four distances
and the track summary, BR-GPS-025..029 and BR-GPS-033, UC-GPS-05/08, §6.2
quality uses, §7 the `trip.trip.read` and `trip.trip.review` rows, §9 events,
§10.1 the four distances and the selection rule, §10.4, §10.5 worked table,
SCR-GPS-07, §12 acceptance 8); [business rules](../04-business-rules.md)
BR-TRUTH-003/004/005 and BR-MONEY-002/008/009/010/011;
[privacy](../11-privacy-security.md) (location and access log);
[validation](../15-validation.md) VAL-27 and the V-DOM/V-MONEY profiles of
[conventions](../00-conventions.md) §6; the [GPS-005](f4-gps005-track-processing.md),
[GPS-012](f4-gps012-location-access-audit.md),
[METER-005](f4-meter005-replacement-rollover.md) and
[METER-006](f4-meter006-current-value-projection.md) evidence; the trip
record, closure, cancellation and own-reading rules (`trip/application`,
`trip/domain.MeterUsage`, the embedded reading's declared instant in
`trip/adapters/intake.go`); the meter `Track.Between` anchors
(`fleet/domain/meter/segments.go`) and the METER-004 supersede rule (a
correction keeps the declaration unless one is given). The previous agent's
drafts (`/data/dev/worktrees/f4-logs/gps006-contract.md`, `gps006-src/`) were
reviewed and reused in part: the route-dependent rule became a port rule, the
evaluation gained the planned and estimated sources, kind-checked inputs, a
closed `DivergenceStatus`, bounds and the anchor-instant rule.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-491..494, BUC-EMP-250..251, EDR-318..320, all
used. Migration 000253 is not used (no persistent state: the signal is
derived at read time, EDR-319).

Rules B-BR-EMP-491 (each distance source stands alone), B-BR-EMP-492
(travelled distance basis), B-BR-EMP-493 (divergence is data for review),
B-BR-EMP-494 (the GPS part is a recorded-track read) and BUC-EMP-250/251 are
in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-006 — Distance sources on a trip and the divergence signal
(2026-11-16)", written before any behaviour. Decisions in
[16-decisions](../16-decisions.md), taken unattended as the recommended
routine choices:

- EDR-318 (coordinator decision): a pure `tracking/domain` evaluation plus a
  server port whose store proves `gps.track.read` through the EMP-GPS-012
  `TrackAuthority.CheckTrackTx` in the read transaction (a permission denial
  withholds the GPS source, any other refusal ends the read), reads the trip
  and the latest GPS-005 summary, takes the odometer from METER-006
  `UsageBetween` and records the `LOCATION_TRACK` access pair before
  answering; no HTTP route (the first `trip.trip.read` route is
  EMP-TRIP-006's). Recorded tension: §7 lists the four distances under
  `trip.trip.read` (numbers only); this task is stricter until the route
  owner decides.
- EDR-319: the odometer distance is accepted only when the usage is known and
  both anchors are readings at exactly the trip's start and finish instants;
  closure, cancellation and an unfinished trip are answered without the
  meter; reason vocabulary; GPS source from the latest summary; default
  thresholds; read-time signal, no event, no table, 000253 unused.
- EDR-320: no Kotlin mirror and no shared vector file (server-only, as
  EDR-298); V-MONEY is met without money (exact bounded `int64` metres,
  integer comparisons, a value only with its basis).

## Shape frozen before behaviour

- `tracking/domain/distance.go`: `ErrDistanceInput`, `SourcesMethod`
  (`trip-distance-sources-v1`), `MaxDistanceMetres` (10¹²), the threshold
  defaults and ranges, `DistanceKind` (`ODOMETER`, `GPS`, `PLANNED`,
  `ESTIMATE`), `SourceStatus` (`OK`, `INSUFFICIENT_DATA`, `WITHHELD`),
  `DistanceReason` (16 closed reasons), `DistanceSource`, `Thresholds`
  (`NewThresholds`, `DefaultThresholds`), `Basis`, `Travelled` (`Value`),
  `DivergenceStatus` (`RAISED`, `NOT_RAISED`, `NOT_EVALUATED`), `Divergence`,
  `Distances`, `EvaluateDistances`.
- `tracking/application/distance.go`: `ErrDistanceDisabled`,
  `DistanceRequest`, `OdometerAnchor`, `OdometerDistance`, `SummaryFacts`,
  `DistanceFacts`, `TrackDistance`, `DistanceView` (all redacted),
  `ReadOdometer` (the only caller of the meter port), `TrackOf`,
  `ComposeDistances`, port `DistanceRepository`, `DistanceService.Read`.
- `tracking/adapters/distance.go`: `DistanceStore` (`Pool`, `Authority`
  — the GPS-012 port, `Trips`, `Meters` — the METER-006 port, `Access`,
  `NewID`) with `ReadDistances`.
- No route, OpenAPI, inventory, matrix, auditor-walk, sqlc, migration,
  composition, Kotlin or Android change; no dependency. The store is not
  composed into `enterprisesuite` because nothing calls it before EMP-TRIP-006.

## RED

The tests were written against compiled stubs (`EvaluateDistances` returning
an empty result, `NewThresholds` refusing everything, `DefaultThresholds` and
`Travelled.Value` zero, `ReadOdometer`/`TrackOf`/`ComposeDistances`/`Read`
returning zero values, `ReadDistances` unavailable).

- Unit (`go test -race ./internal/modules/tracking/...`), log
  `/data/dev/worktrees/f4-logs/gps006-unit-red.jsonl`: 13 new top-level tests,
  13 failing (7 domain, 6 application), no panic; the 39 earlier top-level
  tests (443 subtests) of the tracking packages pass. A first run
  (`gps006-unit-red0-panic.jsonl`) panicked in
  `TestDistanceValuesAreRedacted` on the stub's nil anchor, which stopped the
  later tests of the package; the test now fails with a message when there is
  no composed answer (same expectation). `TestDistanceDivergenceStatesNoMisconduct`
  fails at the stub evaluation; its vocabulary and field-set checks hold by
  the shape alone and are not claimed as RED.
- PostGIS (`-tags=integration -race -run TestTripDistance`), logs
  `gps006-db-red.jsonl`/`.stderr`: 3/3 top-level and 2/2 subtests failing
  (`tracking: unavailable` from the stub store). The run already showed the
  trap on real data: the METER-006 port answers a trip finished without its
  reading with a known 0 m (the start reading at both instants).

## GREEN and REFACTOR

- Unit, log `gps006-unit-green.jsonl`, coverage
  `gps006-unit-green-coverage.out`: 52 top-level (443 subtests) pass in the
  tracking packages; the domain evaluation is 100 % covered, the application
  `TrackOf`, `ComposeDistances` and `Read` 100 % and `ReadOdometer` 95.5 %
  (the adapter is covered by the PostGIS tests).
- PostGIS focused, log `gps006-db-green1.jsonl`: 3/3 top-level and 7/7
  subtests pass under `-race` on the first GREEN run.
- REFACTOR: the context handling of `ReadOdometer` was simplified (a
  cancelled read returns the context error before any mapping); unit tests
  rerun green.

Named negatives mapped to tests:

- Mixed sources: `TestDistanceMixedSourcesAreRefusedOrKeptApart` (planned
  180 000 m and estimate 181 000 m never become travelled distance and never
  enter the signal; a GPS figure in the odometer's place, an odometer in the
  GPS place, a planned distance in the odometer's or estimate's place, a
  quality on a non-GPS source, a `WITHHELD` non-GPS source and every reason
  in another source's place are `ErrDistanceInput`; the travelled value is
  exactly one source's value); `TestDistanceWorkedExamples` (every §10.5 row
  with and without planned/estimate beside it, sources returned unchanged);
  `TestReadOdometerNeverTurnsAnAbsentReadingIntoZero` (an hour-meter answer,
  another asset's or another window's answer is an outage, never a figure;
  BR-MONEY-009); `TestComposeDistancesKeepsTheSourcesApart` (planned and
  estimate are `NO_ROUTE_PLAN`/`NO_ROUTE_COMPUTATION`, another asset's
  odometer refused).
- Absent basis as zero: `TestDistanceAbsentBasisIsNeverZero` (every odometer
  reason with every GPS state: `INSUFFICIENT_DATA` with the reason,
  `Travelled.Value` not ok; a number behind an unknown status, an unknown
  without a reason or with an unknown reason, a negative or above-ceiling
  value refused; a measured 0 m is a value with its basis);
  `TestReadOdometerAsksOnlyForAFinishedTripsWindow` (in progress, paused,
  cancelled and closed trips are answered without asking the meter);
  `TestReadOdometerNeverTurnsAnAbsentReadingIntoZero` (the port's 0 m from
  the start reading at both instants is `NO_FINAL_READING`; a mid-trip or
  earlier anchor is `NO_FINAL_READING`/`NO_START_READING`; `NO_READING_BEFORE`,
  `METER_GAP`, `NO_METER` and `METER_UNAVAILABLE` are reasons);
  `TestTrackOfStatesWhyThereIsNoGPSDistance` (`NO_TRACK`, `NOT_PROCESSED`,
  `NO_GPS_DISTANCE` with the summary's quality and coverage; inconsistent
  summaries refused); PostGIS `TestTripDistanceWithoutAFinalReadingIsNeverZero`
  (real trip finished without its reading: the port answers 0 m known; the
  distance port answers `NO_FINAL_READING` naming only the start reading, the
  precise track becomes the `GPS` basis, and without the track grant the
  travelled figure is `INSUFFICIENT_DATA`, never 0).
- Divergence described as misconduct: `TestDistanceDivergenceStatesNoMisconduct`
  (no status, basis, kind, reason or method token names fraud, theft,
  misconduct, suspicion, cheating, loss, guilt, violation or a person; the
  signal's fields are exactly status, reason, both values, difference and
  thresholds; a raised signal carries its evidence).
- Exact integers and thresholds (V-MONEY): `TestDistanceDivergenceBoundaries`
  (10 % exactly and 2 000 m exactly not raised, one metre above raised, both
  directions, measured zero, configured 500 m/5 %, the 10¹² m ceiling in both
  places without overflow), `TestDistanceThresholdsDefaultsAndRanges`
  (defaults, every range edge, out-of-range refused in construction and in
  evaluation), `TestDistanceVal27` (VAL-27 with 5 %: 28 000 m of 290 000 m
  raised, 6 000 m not; no usable track is `INSUFFICIENT_DATA`).
- Other organization → not found, unknown trip, invalid proof:
  PostGIS `TestTripDistancesComposeTheMeterAndTheTrackUnderTheTrackGrant/foreign-unknown-and-unproven-reads-not-found`
  (a second activated organization's owner naming this trip under their
  scope and an unknown trip are `NOT_FOUND`; a session that is not the
  reader's is `ErrTrackUnauthorized`; another member's membership is
  `NOT_FOUND`; nothing recorded).
- Access audit exactly as EMP-GPS-012: the same PostGIS test (every served
  answer records one `LOCATION_TRACK` `ISSUED` and one `SERVED`, including the
  untracked and the recording states; a FINANCE member with `trip.trip.read`
  and no `gps.track.read` gets the GPS source `WITHHELD` with no session,
  quality, coverage or revision, the divergence `GPS_WITHHELD` and no record;
  the AUDITOR's recorded-track grant is served and recorded;
  `a-failed-access-record-serves-nothing`: with the insert revoked the owner
  gets `ErrAccessUnrecorded` with nothing returned and no row, a withheld
  answer still works, and after the grant returns the read is recorded).
- Missing meter readings → missing, real data: the PostGIS test above (in
  progress `TRIP_NOT_FINISHED`, untracked `NO_TRACK`, recording
  `NOT_PROCESSED`) and `TestTripDistanceWithoutAFinalReadingIsNeverZero`.
- Divergence on real data: the first PostGIS test (odometer corrected through
  the real METER-004 decision to exactly 1 500 m above the processed GPS
  distance: `NOT_RAISED`, difference 1 500; corrected again to 8 000 m above:
  `RAISED` with both values, the new anchor at the same finish instant and a
  higher projection revision).
- Concurrency while a reading changes: `TestTripDistanceAndReadingCorrectionSerialiseInBothOrders`
  (a distance read and a correction of the trip's final reading queued behind
  a held meter lock, both orders: correction first → the answer shows
  280 000 m, the new reading and a higher revision; read first → 290 000 m,
  the original reading and the same revision).

## Validation

Commands from `backend/` with toolchain go1.27.4 (`gps006-env.sh`),
`GOCACHE=/data/dev/cache/f4-go-build`, the shared PostGIS (isolated child
databases) and a throwaway MinIO project `f4-gps006-s3` (`down -v`
afterwards):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...     # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... ./internal/modules/organization/adapters/...
go test -race -count=1 ./internal/modules/tracking/...
go test -tags=integration -race -count=1 -p 2 -json -timeout=120m ./internal/modules/organization/adapters ./internal/modules/tracking/... \
  ./internal/modules/fleet/... ./internal/modules/trip/... ./internal/modules/orgaudit/... ./internal/modules/orgaccess/...
go test -count=1 -timeout=45m -p 2 ./...
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
```

- Static: `gps006-build-vet.log` (toolchain gofmt empty on `./internal ./cmd
  ./db`, build, vet both tags) and `gps006-static.log` (staticcheck plain and
  `-tags=integration`) are clean; `sqlc vet` and `sqlc diff` clean (no query
  or `sqlc.yaml` change; the store uses parametrized SQL like GPS-012).
- Race/integration union with MinIO, logs `gps006-db-union.jsonl`,
  `.stderr` (empty), `.exit` (0), `.start` (15:01–15:52Z): 21 packages (20
  pass, 1 without integration tests), 1 098 top-level and 9 083 named
  subtests, 0 failures. It includes the three new tests, the organization
  adapters (3 079 s under a loaded machine) with the AUTH-011 auditor walk,
  the 300-cell authorization matrix, the GPS-012 track read, the GPS-005
  processing, the METER-006 usage and concurrency tests and the trip suites.
  No route changed: the walk (164/33/131) and the matrix (300 cells,
  121/179) ran unchanged here, and the untagged `cmd/api` inventory test
  (164 normal / 30 routed / 1 signed / 54 not routed, specials 26; 20 signed
  owners) ran in the untagged suite below.
- Untagged `go test -count=1 -timeout=45m -p 2 ./...`, log
  `gps006-untagged-all.log`, exit 0: 230 packages ok (including `cmd/api`
  with the route inventory), 86 without tests, 0 failures.
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens,
  problems 0.
- `git diff --cached --check` clean; `gitleaks stdin --redact` on the staged
  diff: no leaks. The only secret-looking strings are the redaction test's
  synthetic identifiers.
- No Kotlin, Android or emulator run: no shared domain or app change
  (EDR-320).

## Retained obligations

- EMP-TRIP-006 composes the port behind the first `trip.trip.read` route with
  its structural, self and AUDITOR semantics, its OpenAPI view and the
  inventory, matrix and auditor-walk updates; whether the GPS figure may be
  served under `trip.trip.read` (numbers only, gps-navigation §7) instead of
  `gps.track.read` is that owner's decision (EDR-318).
- Organization divergence thresholds (SCR-GPS-09) need a settings owner; the
  domain already validates the ranges.
- The divergence outcome (UC-GPS-08, `trip.trip.review`), the
  `gps.divergence.detected` event and re-evaluation consumers of meter
  changes; the signal is derived at read time meanwhile.
- The planned distance and the engine estimate stay `INSUFFICIENT_DATA`
  until the route plan and route computation owners (EMP-ROUTE) record them.
- Consumers of the basis (consumption, cost, indicators) must use
  `Travelled.Value` and keep the basis label "por GPS".
- The GPS source names the latest stored summary revision; a summary that is
  older than a later batch shows its own revision and instant until the
  pending job runs (GPS-005 retained items unchanged).
- Production grants (runtime `SELECT` on `org_tracking_sessions`,
  `org_tracking_summaries`, `org_meter_current_values` and `SELECT, INSERT`
  on `org_sensitive_access_events`) are not seeded, as for every enterprise
  table.
- Real-device acceptance stays OWED (EDR-271). No M4 or F4 acceptance is
  claimed.

## Shared files touched

`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md` (section appended),
`docs/enterprise/16-decisions.md` (EDR-318..320 appended at the end of the
table), `docs/enterprise/backlog/ep07-gps-navigation.md` (state and
checkpoint). No route inventory, OpenAPI, matrix, auditor walk,
check-manifest or migration change.
