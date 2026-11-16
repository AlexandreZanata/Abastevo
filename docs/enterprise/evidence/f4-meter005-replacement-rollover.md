# EMP-METER-005 — Meter replacement, reset and rollover with cumulative usage

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2 prerequisite of EMP-GPS-006 (module METER, P1, type DOM, size M)
on `codex/enterprise-f4-meter005`, opened at the F4 head `1a566b45` and
fast-forwarded to `c7e407e6` (TRIP-022/023, migration 000249) before any
shared route file was touched. It keeps distance and hours correct across a
meter replacement, a reset and a rollover: a pure segment model shared by Go
and Kotlin, the rollover candidate flag, and the replace/reset/confirm
commands with their stored implied usage. Synthetic data only; no Android
screen, panel, job or release change.

Sources read: the EMP-METER-005 and EMP-METER-006 blocks of
[ep05](../backlog/ep05-fleet-meters.md); [fleet](../modules/fleet.md) §3
(meter and segment entities), §4 BR-METER-001..026 (007, 018..020 and 022
owned here), §5 UC-METER-05/06/08, §6.5, §7, §8, §10.2..§10.5 (examples C and
D); the METER-001..004 evidence ([001](f2-meter001-reading-domain.md),
[002](f2-meter002-persistence-intake.md),
[003](f4-meter003-reading-validation.md),
[004](f4-meter004-reading-correction.md)); TRIP-001 honest usage and TRIP-015
amendment supersession; EDR-184..189, EDR-236/237.

IDs used: B-BR-EMP-481..486, BUC-EMP-245..247, EDR-311..314 (all of them).
Migration `000251_organization_meter_segments.sql` (the backlog said "none";
closing a segment and removing a meter need guarded updates of tables whose
statement triggers refused every update, so the pre-allocated number was
used).

## Rules frozen before behaviour

Observed before any change: `org_meters` and `org_meter_segments` refused
every UPDATE (statement `org_asset_keep_history`), so `removed_at` and
`closed_at` were never set; segments admitted only `INSTALLATION`; the
history cause list had no rollover; `org_meter_flags_canonical` knew four
flags and the validation-v1 case `unknown-flag` uses `POSSIBLE_ROLLOVER` as
invalid input; every segment-keyed query filtered the recorded
`org_meter_readings.segment_id`; `UNIQUE(org_id, meter_id, operation_id)`
allows one reading per meter per operation; a device reading naming a
non-open segment is held `RETRY`/`DEPENDENCY_PENDING`.

Rules B-BR-EMP-481, B-BR-EMP-482, B-BR-EMP-483, B-BR-EMP-484, B-BR-EMP-485,
B-BR-EMP-486, BUC-EMP-245, BUC-EMP-246, BUC-EMP-247 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-METER-005 — Meter replacement, reset and rollover with cumulative usage".
Decisions EDR-311, EDR-312, EDR-313, EDR-314 are recorded in
[16-decisions](../16-decisions.md).

Refinements recorded before the persistence RED, each found while writing the
migration: (1) a reset cannot store both its before and after values as
readings of one meter under one operation (`UNIQUE(org_id, meter_id,
operation_id)`), so the before value is the closed segment's closing value
and only the after value is a reading (B-BR-EMP-481/485, EDR-313); (2) the
backlog risk asks each command to show *and store* the implied usage, so the
closure stores it with the reading it was measured from (re-proved by the
guards). One refinement after the first upgrade run, before the fix: the
segment-keyed queries first called the new SQL function, which older images
of the upgrade rehearsals do not have; the queries now inline the same rule
over 000198 columns, and the rule became "a `METER_REMOVAL` reading counts in
the segment it was recorded in" (equivalent for every reading the commands
write, and needs no closure column) (B-BR-EMP-481, EDR-311).

## Implementation

- Pure model (Go `fleet/domain/meter/segments.go` + `validation.go`, Kotlin
  `MeterSegments.kt` + `MeterValidation.kt`): `PossibleRollover` /
  `ReadingFlag.POSSIBLE_ROLLOVER` as the last canonical flag, admitted only
  with a modulus; modulus-aware `CheckMeter`/`ReevaluateMeter`
  (`checkMeter`/`reevaluateMeter`) with the exact wrap check
  (`(M − v₁) + v₂` in `math/big`/`BigInteger`); the modulus-free entry points
  are unchanged. `NewTrack`/`MeterTrack.of` validates a chain of spans and
  VALID anchors; `Cumulative` (U(t), E(t), anchor) and `Between`
  (usage(t1, t2), both anchors) return `NO_READING_BEFORE`/`METER_GAP` and
  never interpolate; `PlanReplacement`, `PlanReset`, `PlanRollover` decide
  the command preconditions and the implied usage.
- Shared oracle `contracts/testdata/enterprise/meter/segments-v1.psv`
  (143 cases: CHECK_WRAP 31, REEVALUATE_WRAP 13, CUMULATIVE 35, USAGE 19,
  REPLACE 17, RESET 12, ROLLOVER 16; SHA-256
  `296afeb474264ba4305004cbcc13c56761c192eca8c7ab9b9eede6e9cf15d21f`) with
  its grammar in `SEGMENTS.md`. Expectations were produced by an independent
  Python model written from the contract rules
  (`/data/dev/worktrees/f4-logs/meter005-oracle/build.py`, vectors in
  `vectors.py`), with examples C (1 944 000 / 754 000 / 184 944 000 m),
  D (170 000 / 1 000 120 000 m), a reset and the unknown final value asserted
  by hand inside the generator. Both consumers assert digest, counts, unique
  IDs, every operation, `POSSIBLE_ROLLOVER`, both reasons and `INVALID`.
- Migration 000251: widened `org_meter_flags_canonical`; history cause
  `ROLLOVER` (person, `VALID`); segment kinds `RESET`/`ROLLOVER` with an
  opening operation; closure columns (kind, value or unknown, removal
  reading, operation, previous reading, implied usage) with a closure CHECK
  and same-organization foreign keys; `org_meters.removal_operation_id`;
  `org_meter_reading_state` and `org_meter_reading_segment` SQL functions;
  the keep-history statement triggers kept for DELETE/TRUNCATE; row guards
  admitting only the first removal and closure with every other column
  unchanged and re-proving the closure (instant after the opening, modulus
  of a rollover, removal reading, latest VALID reading and implied usage,
  nothing live or VALID left at or after the boundary); an opening guard for
  RESET/ROLLOVER segments; deferred chain checks on segments and meters; the
  000216 reading-state guard with one clause added (`POSSIBLE_ROLLOVER` needs
  a modulus) and three changed (`CONFIRMED` refuses it, `ROLLOVER` needs it
  and its segment, supersession compares derived segments).
- sqlc (fleet stanza + 000251): the five segment-keyed reads use the derived
  segment; `MeterSegmentChain`, `MeterReadingIsAnchor`, `CloseMeterSegment`,
  `InsertBoundarySegment`, `RemoveMeter`, `MeterBoundaryByOperation`.
- Store (`fleet/adapters/meter_boundaries.go`, `meter_corrections.go`,
  `meter_validation.go`): intake, correction and re-evaluation classify with
  the modulus; `ChangeMeter` (root, asset and meter locks; segment count as
  revision; database clock; planner; one audited transaction; replay by the
  operation's boundary row plus the audit event, rebuilt from rows); the
  `CONFIRM_ROLLOVER` decision (closure at the modulus, ROLLOVER segment on
  the reading, cause `ROLLOVER`, re-evaluation of the new segment, boundary in
  the result); every segment opening/closing reading refuses decisions.
- Application: `MeterChangeCommand` (digest over every field),
  `MeterBoundaryResult`, `MeterCorrectionService.Change` re-proving the live
  decision before and after the store and the stored result shape;
  `MeterConfirmRollover`.
- HTTP: `POST …/meters/{meter_id}/change` in `RegisterMeterCorrections`
  (meter.reading.correct, write, fresh, revision); the decide route accepts
  `CONFIRM_ROLLOVER` and returns `boundary`; counts are decimal strings.
- Contract: one OpenAPI path, schemas `EnterpriseMeterChangeRequest`,
  `EnterpriseMeterChange`, `EnterpriseMeterBoundary`,
  `EnterpriseMeterImpliedUsage`, `EnterpriseMeterCount`; the flag enum, the
  decision enums and the reading-state flag bound widened; inventory
  `meter.reading.correct` +1 route (normal 162 → 163); router path list +1;
  AUDITOR walk 163 routes / 33 positive / 130 FORBIDDEN.

## Acceptance and negatives

| Item | Test(s) |
|---|---|
| AC1 example C: 1 944 000 m and 754 000 m across the replacement | oracle `c-cumulative-after-replacement`, `c-usage-across-replacement` (Go + Kotlin); PostGIS `TestMeterSegmentExampleCReplacementThroughTheStore` (stored chain → track) |
| AC2 example D: `POSSIBLE_ROLLOVER`, then 170 000 m | oracle `d-wrap-candidate`, `d-usage-across-rollover`, `d-rollover-confirm`; PostGIS `TestMeterSegmentExampleDRolloverConfirmation`; HTTP `TestMeterBoundariesThroughComposedCompanyHTTP` |
| AC3 never applied without a person | `TestMeterSegmentExampleDRolloverConfirmation` (latest stays S1, one segment; plain `CONFIRM` refused); guard cases "rollover without its segment", "rollover flag confirmed" |
| AC4 unknown final value → `INSUFFICIENT_DATA`/`METER_GAP` | oracle `gap-*`, `c-replace-unknown-final`; PostGIS `TestMeterSegmentUnknownFinalValueMakesAGap`; HTTP replacement with `final_unknown` |
| AC5 monotonicity inside each segment only | oracle `reeval-reset-*`; PostGIS `TestMeterSegmentResetReturnsLaterReadingsToValid` (5 000 VALID after 1 200 000 in another segment) |
| Installing a meter while one of the kind is in service | `TestMeterSegmentExampleCReplacementThroughTheStore` (`METER_ALREADY_IN_SERVICE` after the replacement) |
| Opening value above the modulus | oracle `replace-opening-at-new-modulus`; PostGIS refusals; HTTP 422 |
| Confirming a rollover on a meter without a modulus | oracle `rollover-without-modulus`; PostGIS (second fixture) |
| Reset whose before value is below the last valid reading | oracle `reset-before-below-latest`; PostGIS reset refusals |
| Concurrent replacement and intake | `TestMeterSegmentConcurrentReplacementAndIntakeInBothOrders` (both orders queued behind a held root lock: the replacement measures from the committed reading, or the reading is NOT_FOUND on the removed meter and nothing is stored) |
| Replay and digest reuse | example C and D tests (byte-equal results, `OPERATION_ID_REUSED`); HTTP reset replay |
| Stale segment count, foreign meter, removed meter | refusals tests (`CONFLICT`, `NOT_FOUND`); HTTP 409 and 404 |
| DRIVER, supervisor of another unit, another organization | HTTP 403, 403, 404 |
| Guards, forced RLS, history | `TestMeterSegmentSchemaGuardsTheChain` (15 refused writes, closure not rewritable, foreign organization sees and writes nothing) |
| Populated upgrade 000247 → 000251 | `TestMeterSegmentsPopulatedUpgradeFailureRecoveryAndPreviousImage` (injected failure leaves schema, guard and data; recovery once; no-op replay; previous image compatible; legacy readings count in their recorded segment; legacy meter reset; remaining chain applies) |

## Validation — RED → GREEN and gates

Environment: Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
shared PostGIS `127.0.0.1:32775` through orgtest child databases, throwaway
MinIO `f4-meter005-s3`, JDK 17 under the shared Gradle lock. Logs in
`/data/dev/worktrees/f4-logs/meter005-*`.

- Domain RED (stubs returning a non-domain error; a first run whose stubs
  returned `ErrInvalid` let 57 INVALID vectors pass for the wrong reason and
  was discarded) → `meter005-go-domain-red.jsonl`: 2 of 25 top-level fail,
  143/143 segment vectors fail, and validation-v1 `unknown-flag` fails
  because the flag order already listed `POSSIBLE_ROLLOVER` (fixed in GREEN
  by admitting it only with a modulus). Kotlin RED (`TODO()` stubs)
  → `meter005-kotlin-red.log`: 575 tests, 144 failures (the same 143 + 1).
- Domain GREEN: Go `-race` with coverage → `meter005-go-domain-green.jsonl`:
  26 top-level / 602 subtests pass, package 94.7% (`segments.go` 82–100% per
  function; the uncovered lines are int64 overflow refusals unreachable with
  bounded values). Kotlin meter tests 575/0; enterprise suite
  `meter005-kotlin-green.log`: 4 437 tests, 0 failures/errors/skips.
- PostGIS RED (migration applied, stubbed store, modulus-free intake)
  → `meter005-db-red.jsonl`: 6/6 top-level, 15/15 subtests fail. The schema
  and upgrade tests were written after this GREEN: no RED is claimed for them.
- GREEN corrections, each recorded before the fix: the reading codec compared
  the recorded segment with the view (queries now return the requested,
  derived segment); the upgrade run exposed the function dependency above;
  the guard test's rollover fixture used a modulus too far from its values for
  a plausible wrap (fixture data corrected); `TRUNCATE org_meter_segments` is
  refused by PostgreSQL (`0A000`, referenced by foreign keys) before the
  statement trigger, so that case asserts `0A000` and the trigger coverage is
  asserted from `pg_trigger`.
- Expectation changes in existing tests: the METER-004 contract test used
  `POSSIBLE_ROLLOVER` as its example of an unknown flag; B-BR-EMP-483 makes it
  real, so it now asserts the flag accepted and uses a made-up flag for the
  refusal. The Kotlin validation-v1 consumer's "every flag has a vector"
  loop excludes `POSSIBLE_ROLLOVER`, which segments-v1 covers.
- PostGIS GREEN focused → `meter005-db-green.jsonl`, `meter005-db-schema.jsonl`
  and `meter005-db-focused.jsonl`: every `TestMeterSegment*`, the composed
  HTTP test, METER-003/004 validation/correction, the 000216 upgrade and
  schema tests and the TRIP-015 amendment tests pass; the AUDITOR walk passes
  with MinIO.
- Affected race union on the merged tree (`c7e407e6` + this task), with the
  throwaway MinIO: `go test -json -race -tags=integration -count=1 -p 2
  -timeout 120m ./internal/modules/organization/adapters
  ./internal/modules/fleet/... ./internal/modules/trip/...
  ./internal/modules/tracking/... ./internal/modules/fueling/...
  ./internal/modules/orgsync/... ./internal/modules/orgexport/...
  ./internal/platform/... ./cmd/api/...` → `meter005-union.jsonl`: 45
  packages (41 pass, 3 without integration tests skipped), 1 457 top-level /
  9 819 subtests, one failure: the untouched
  `TestTrackingIntakeOpensOnlyUnderAPolicyWithItsNoticeInForce` (a final
  refusal expected under a 54-minute contended run; organization adapters
  3 245.8 s), which passes alone and `-count=3` (`meter005-union-rerun.jsonl`,
  3/3). No meter, fleet or trip test failed.
- Untagged `go test -count=1 -timeout=45m -p 2 ./...` →
  `meter005-untagged-all.log`: 230 packages ok, 86 without tests, 0 failures.
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean;
  toolchain gofmt empty on touched packages; staticcheck (go1.27.1) plain on
  fleet, apicontract and cmd/api and `-tags=integration` on organization
  adapters and fleet: no findings; `sqlc vet`, `sqlc generate`, `sqlc diff`
  clean; `git diff --check` clean; `check_plan.py` 0 problems.
- Secret/PII review of the staged diff: synthetic identifiers, labels and
  reasons only; the MinIO credentials are the documented local validation
  values in the command environment only.

## Retained (not part of this task's acceptance)

- The maintained current-value projection, the usage-between-instants port
  and the recalculation consumers (EMP-METER-006).
- SCR-METER-04 and the history of closed segments and removed meters
  (EMP-METER-007): the history route still reads the open segment and a
  removed meter answers `NOT_FOUND`.
- Device readings that name a segment closed by a boundary keep the
  `RETRY`/`DEPENDENCY_PENDING` outcome of a non-open segment; re-targeting
  late readings to the segment holding their instant belongs to the offline
  intake (fleet §8).
- Undoing a confirmed rollover or a recorded boundary (no correction path is
  specified); a wrong boundary stays visible with its stored implied usage.
- TRIP-001's in-trip usage still compares the recorded segments of its two
  readings (`NOT_MEASURED` across a boundary); cross-segment trip distance is
  EMP-GPS-006 through EMP-METER-006.
- Production grants (backend README).

## Shared files touched

`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md` (section appended),
`docs/enterprise/16-decisions.md` (EDR-311..314 appended),
`docs/enterprise/backlog/ep05-fleet-meters.md` (state and checkpoint),
`backend/README.md`, `backend/sqlc.yaml` (fleet stanza),
`contracts/openapi/v1.yaml`, `contracts/testdata/enterprise/authorization-routes-v1.json`,
`backend/cmd/api/authorization_inventory_test.go` (normal 163),
`backend/cmd/api/enterprise_router_test.go`,
`backend/internal/modules/organization/adapters/auditor_matrix_http_integration_test.go`
(163/33/130), `backend/internal/modules/organization/adapters/meter_store_integration_test.go`
(grants when 000251 exists), `backend/internal/platform/apicontract/meter_corrections_test.go`.
The authorization-matrix test needed no change (`meter.reading.correct` keeps
its representative); check-manifest unchanged (no new package).
