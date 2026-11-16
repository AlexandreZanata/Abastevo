# EMP-METER-006 — Current-value projection and usage-between-instants query

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded task (module METER, F3, P1, type DB, size S) on
`codex/enterprise-f4-meter005`, after EMP-METER-005 (`d3d755db`). It gives
other modules one maintained current value per meter and one usage answer
between two instants that names the readings it used, and an operator
rebuild. Synthetic data only; no route, Android, panel or release change.

Sources read: the EMP-METER-006 block of [ep05](../backlog/ep05-fleet-meters.md);
[fleet](../modules/fleet.md) BR-METER-006/007/016, UC-METER-08, §10.3 and §12
(acceptance 10 and 14); BR-TRUTH-003/004; [09-architecture](../09-architecture.md)
§5 (read models); the EMP-METER-004 recalculation event (EDR-189) and the
EMP-METER-005 segment model ([evidence](f4-meter005-replacement-rollover.md));
EMP-TRIP-015 supersession (EDR-237); the 000171 `SECURITY DEFINER` trigger
precedent; `cmd/ops`.

IDs used: B-BR-EMP-487..490, BUC-EMP-248..249, EDR-315..317 (all of them).
Migration `000252_organization_meter_current_values.sql`.

## Rules frozen before behaviour

Observed before any change: every reading fact passes through
`org_meter_readings`, `org_meter_reading_states`, `org_meter_segments` or the
000251 removal update, each guarded and locking the meter row `FOR UPDATE`;
`WithOrgTx` pins READ COMMITTED on purpose; forced RLS hides every
organization from a migration without a scope; `cmd/ops` denies anonymous
operators; `meter.usage.recalculated` has no production consumer.

Rules B-BR-EMP-487, B-BR-EMP-488, B-BR-EMP-489, B-BR-EMP-490, BUC-EMP-248,
BUC-EMP-249 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-METER-006 — Current-value projection and usage-between-instants
query"; decisions EDR-315, EDR-316, EDR-317 in [16-decisions](../16-decisions.md).
One refinement before behaviour (drafted while the METER-005 union ran): the
port first read in a repeatable-read snapshot, but `WithOrgTx` is pinned to
READ COMMITTED for its write-skew rule, so B-BR-EMP-490 and EDR-316 take a
share lock of the meters instead. One defect found by the frozen concurrency
test and fixed: the locking statement also joined the projection, and in READ
COMMITTED a row joined by a statement that waited on a lock keeps the
statement's original snapshot, so the answer mixed the old revision with the
corrected readings; the lock and the reads are now separate statements.

## Implementation

- Migration 000252: forced-RLS `org_meter_current_values` (organization +
  meter key; same-organization keys to the meter, the open segment and the
  current reading; shape CHECK), PUBLIC holds nothing;
  `org_meter_current_revision` (count of readings, history revisions,
  segments, closures and the removal); `SECURITY DEFINER` functions
  `org_meter_current_refresh` (scope check, open segment, latest currently
  VALID reading in the derived segment, upsert) and
  `org_meter_current_source` with `AFTER` row triggers on readings, history
  revisions, segments (insert and update) and meters (update);
  `org_meter_current_rebuild` (scope check, root lock, delete, recompute every
  meter, return the count). No backfill (forced RLS); the operator rebuild
  fills existing organizations.
- sqlc (fleet stanza + 000252): `LockAssetKindMeters` (`FOR SHARE`),
  `AssetKindMeterRevisions`, `AssetKindSegmentChain`, `AssetKindValidReadings`
  (the derived-segment rule inlined, current state).
- Port: `fleet/application.MeterUsagePort`/`MeterUsage` (method
  `meter-usage-v1`); `fleet/adapters.MeterStore.UsageBetween` builds the
  EMP-METER-005 track and answers with `Between`, both anchors (identifier,
  value, instant) and the revision sum; `NOT_FOUND` for another organization
  or no meter of the kind, `VALIDATION_FAILED` for reversed instants,
  unavailable for an unbuilt projection or a chain that is not a valid track.
  `RebuildMeterProjection` runs the rebuild.
- Operator tool: `ops meter rebuild-projection --org <uuid> [--operator <id>]`
  (operator identity required, canonical organization, prints the count).
- No Kotlin change (EDR-317): the app reads no projection and the pure
  formulas are the EMP-METER-005 shared model.

## Acceptance and negatives

| Item | Test(s) |
|---|---|
| AC1 rebuild = incremental with corrections, a rollover and a replacement; "Done when" | `TestMeterProjectionRebuildEqualsIncremental` (supersession, rollover, replacement; revision = counted facts; two rebuilds byte-equal to the maintained rows) |
| AC2 the answer names both anchors and the projection revision | same test (usage 1 200 000 m across rollover and replacement, anchors r1/r6, revision = stored sum); `TestMeterUsageAnchorsGapsAndRefusals` |
| AC3 no anchor before an instant → `NO_READING_BEFORE` | `TestMeterUsageAnchorsGapsAndRefusals` (end anchor still named, usage absent) |
| AC4 never interpolates | `TestMeterUsageAnchorsGapsAndRefusals` (an instant between readings uses the earlier reading) |
| AC5 a correction raises the revision; the event once | `TestMeterUsageAnchorsGapsAndRefusals` (a void raises it by exactly one); the single `meter.usage.recalculated` intent per decision is EMP-METER-004's `TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation`, unchanged and green in the union |
| Usage across organizations | `NOT_FOUND` (same test) |
| Instants in reverse order | `VALIDATION_FAILED` (same test) |
| Asset without a meter of the kind | `NOT_FOUND` (same test, hour meter) |
| Unknown final value between the instants | `METER_GAP` with both anchors (same test) |
| Concurrent correction and query | `TestMeterUsageAndCorrectionSerialiseInBothOrders` (both orders queued behind a held meter lock: wholly before or wholly after, value and revision together) |
| Rebuild run twice | `TestMeterProjectionRebuildEqualsIncremental` |
| Projection written only by the database; forced RLS | `TestMeterProjectionSchemaGuards` (runtime insert/update/delete/refresh 42501, no rows without organization, foreign scope sees nothing and cannot rebuild) |
| Populated upgrade 000251 → 000252 | `TestMeterProjectionPopulatedUpgradeFailureRecoveryAndRebuild` (injected failure leaves schema and data; recovery once; no-op replay; previous image; migrator-owned forced-RLS table and definer functions; unbuilt legacy usage unavailable; rebuild; maintained afterwards; remaining chain applies) |
| Operator argument rules | `cmd/ops` `TestParseMeterArgs` |

## Validation — RED → GREEN and gates

Environment as EMP-METER-005; logs `/data/dev/worktrees/f4-logs/meter006-*`.

- PostGIS RED (migration as written, stubbed port and rebuild returning
  unavailable) → `meter006-db-red.jsonl`: 3 of 4 top-level fail (2 of 6
  subtests); the schema-guard test passes by construction of the migration.
  The `cmd/ops` parser test was written with the parser (no RED claimed);
  the upgrade test was written after GREEN (no RED claimed).
- GREEN → `meter006-db-green1.jsonl` (the mixed-snapshot defect above) and
  `meter006-db-green.jsonl`: 4 top-level / 6 subtests pass; upgrade test
  passes.
- Affected race union (the METER-005 package list plus `./cmd/ops/...`,
  throwaway MinIO) → `meter006-union.jsonl`: 46 packages (41 pass, 3 without
  integration tests skipped), 1 480 top-level / 9 824 subtests, two failures
  under a contended run (organization adapters 4 130.2 s): the untouched
  `cmd/api` `TestPublicProcessIdentityAndSignedWrites` (an upload to the
  local storage timed out) and EMP-TRIP-023's
  `TestTripMissionLinkedStartConsumesItsReservationAndCompletes` (mission
  acceptance unavailable after 50 s). Both pass alone, with every tracking
  intake test (`meter006-union-rerun.jsonl`). No meter, fleet or trip-amendment
  test failed.
- Untagged `go test -count=1 -timeout=45m -p 2 ./...` →
  `meter006-untagged-all.log`: 230 packages ok, 86 without tests, 0 failures.
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean;
  toolchain gofmt empty on touched packages; staticcheck (go1.27.1) plain on
  fleet and `cmd/ops`, `-tags=integration` on organization adapters: no
  findings; `sqlc vet`, `sqlc generate`, `sqlc diff` clean; `git diff --check`
  clean; `check_plan.py` 0 problems. No Kotlin or OpenAPI change.
- Secret/PII review of the staged diff: synthetic identifiers only.

## Retained (not part of this task's acceptance)

- Consumers of the port: consumption intervals (fueling epic), maintenance due
  computation, indicators; EMP-GPS-006 is the first consumer.
- Production composition of `meter.usage.recalculated` consumers (EDR-189):
  the projection needs none.
- Running `ops meter rebuild-projection` for every existing organization after
  deployment, and the production grants (backend README).
- The current-value reads of the reference feed and the panel still compute
  from readings (they also serve images older than 000252).
- Capacity: the refresh recounts a meter's facts per change; no measurement is
  claimed.

## Shared files touched

`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md` (section appended),
`docs/enterprise/16-decisions.md` (EDR-315..317 appended),
`docs/enterprise/backlog/ep05-fleet-meters.md` (state and checkpoint),
`backend/README.md`, `backend/sqlc.yaml` (fleet stanza), `backend/cmd/ops/main.go`
(dispatch and usage), `backend/internal/modules/organization/adapters/meter_store_integration_test.go`
(projection read grant when 000252 exists). No route, OpenAPI, inventory,
matrix or check-manifest change.
