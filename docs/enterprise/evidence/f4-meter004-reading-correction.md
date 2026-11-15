# EMP-METER-004 — Reading correction and void with audit

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2 prerequisite of F4 (module METER, P1, type API, size S) on
`codex/enterprise-f4-meter003`, built on EMP-METER-003 (`2669080c`, migration
`000216`). It lets an authorised person supersede, void or confirm one meter
reading with a reason, lists the suspect readings and the full history with
chains, and re-evaluates suspect neighbours in the same audited transaction.
Synthetic data only; no Android, panel screen, job or release change.

Sources read: the EMP-METER-004 block of
[ep05](../backlog/ep05-fleet-meters.md); [fleet](../modules/fleet.md) §4
BR-METER-011..016 and 026, §5 UC-METER-03/04/07, §6.5, §7 (permissions and
scopes), §8 (corrections online only), §10.4 example B, §10.5
(`READING_ALREADY_SUPERSEDED`); BR-AUD-001 and BR-AMD-001..006 of
[04-business-rules](../04-business-rules.md); the catalog v4 row
`meter.reading.correct O O O O - - - - -`; the EMP-METER-003
[evidence](f4-meter003-reading-validation.md) (EDR-184..186); the PLAT-010
outbox ([evidence](f1-plat010-transactional-events.md): producers compose
real consumers before publishing); the PLAT-016 amendment kernel (pure, no
consumer); AUTH-006 structural scopes (EDR-160/161); the FLEET-006 and
DOC-003 route precedents.

IDs used: B-BR-EMP-286..291, BUC-EMP-145..147, EDR-187..189 (all of them).
Migration: none. `000216` was used by EMP-METER-003 and already carries the
person causes (`CONFIRMED`, `VOIDED`, `SUPERSEDED`), the actor, the reason and
the supersession link (EDR-184), so this task needs no schema change.

## Rules frozen before behaviour

Observed before any change: `meter.reading.correct` was routed only by the
OFFL-008 held-review routes; the AUTH-006 asset structure resolver admitted
`fleet.asset.*` only; `FleetRecords` admitted only fleet policies; the audit
vocabulary had `meter.installed` only; no consumer of
`meter.usage.recalculated` exists and the orgevent catalog refuses a
definition without consumers; fuelings are `SUBMITTED` only (no approval
producer); the reading-list export read `org_meter_readings.state`.

Rules B-BR-EMP-286, B-BR-EMP-287, B-BR-EMP-288, B-BR-EMP-289, B-BR-EMP-290, B-BR-EMP-291, BUC-EMP-145, BUC-EMP-146, BUC-EMP-147 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-METER-004 — Reading correction and void with audit" (frozen before behaviour).

Decisions EDR-187, EDR-188, EDR-189 are recorded in [16-decisions](../16-decisions.md).

## Implementation

- Application (`fleet/application/meter_corrections.go`): `MeterDecisionCommand`
  (operation, asset, meter, reading, expected revision 0..999 999, decision
  `SUPERSEDE`/`VOID`/`CONFIRM`, reason 1..280 without controls; supersession
  value of the stated kind and optional declaration) with a digest binding
  every field; `MeterCorrectionService.Decide/History/Suspects` proving the
  live web decision of the exact policy before and after the store, and
  re-proving the stored result shape; errors `ErrReadingAlreadySuperseded`,
  `ErrAmendmentRequired`; actions `MeterCorrectAction`/`MeterCorrectReadAction`
  mapped to `meter.reading.correct`.
- Store (`fleet/adapters/meter_corrections.go`): under the root, asset and
  meter locks, refusal order NOT_FOUND → `READING_ALREADY_SUPERSEDED` →
  opening anchor (`VALIDATION_FAILED`) → approved parent
  (`AMENDMENT_REQUIRED`) → stale revision (`CONFLICT`) → confirm of a
  non-suspect (`CONFLICT`) or over a monotonicity flag (`VALIDATION_FAILED`);
  then one audited change (`meter.reading.corrected|voided|confirmed`, target
  `meter_reading`, command digest, no free text) that writes the history rows
  and, for `SUPERSEDE`, the new reading (original origin, `TYPED`, deciding
  member, web session) classified against the series with the original voided
  and re-evaluated in memory so its revision 1 is the final verdict; other
  changed suspects get one `REEVALUATION` revision under the operation.
  Replays are detected by the operation's history rows and proved by the audit
  event (else `OPERATION_ID_REUSED`); the result is always rebuilt from the
  operation's rows, so fresh and replayed answers are identical. Optional
  `MeterRecalculation` enqueues one `meter.usage.recalculated` intent per
  composed consumer (target the decided reading, revision its new revision).
  History and suspect list read the open segment in series order with current
  state, flags, note, revision, both supersession links, the latest decision
  reason and (suspect list) the valid neighbours.
- Parent port: `fueling/adapters.Store.ApprovedParentTx` (review state of the
  fueling carrying the reading; sqlc `FuelingReviewStateByReading`).
- Authority: `MeterAuthority` admits the two actions on the web path only;
  the AUTH-006 asset structure resolver admits `meter.reading.correct`;
  `FleetRecords` admits the meter-correction policies.
- HTTP (`fleet/adapters/http/meter_corrections.go`): three routes under the
  asset — `…/meters/{meter_id}/readings/list` (`fleet.asset.read`),
  `…/readings/suspect/list` (`meter.reading.correct`),
  `…/readings/{reading_id}/decide` (`meter.reading.correct`, write, fresh,
  revision) — with cursors bound to route, asset, meter, filter and limit;
  `READING_ALREADY_SUPERSEDED` and `AMENDMENT_REQUIRED` registered in `orgapi`
  (409). Composition in `enterprisesuite` with the fueling store as parent
  port and no recalculation consumer.
- Contract: 3 OpenAPI paths and 7 schemas (`EnterpriseMeterHistoryRequest`,
  `EnterpriseMeterFlag`, `EnterpriseMeterHistoryItem`,
  `EnterpriseMeterHistoryPage`, `EnterpriseMeterDecisionRequest`,
  `EnterpriseMeterReadingState`, `EnterpriseMeterDecision`), the two error
  codes in the enterprise enum; route inventory `fleet.asset.read` +1 route,
  `meter.reading.correct` +2 routes (normal 110 → 113, routed permissions
  unchanged at 21); full-router path list +3; AUDITOR walk: the history is a
  documented positive read (24 → 25 positive, 86 → 88 refused, 113 routes),
  the walk supplies `{reading_id}`.
- Export: the RPT-002 reading list exports the state as of `as_of` from the
  history (latest revision recorded at or before `as_of`, else the recorded
  classification) and accepts the `VOIDED` filter.

## Acceptance and negatives

| Item | Test(s) |
|---|---|
| AC1 two rows, original `VOIDED` and linked, correction current | `TestMeterCorrectionSupersedesOnceAndReevaluatesNeighbours`; composed `TestMeterCorrectionExampleBThroughComposedCompanyHTTP` (history chain both ways, latest = correction) |
| AC2 second correction → `READING_ALREADY_SUPERSEDED` | same two tests (store error and HTTP 409 code) |
| AC3 monotonicity flag cannot be confirmed, rate flag can with a reason | `TestMeterCorrectionConfirmVoidAndRefusals` |
| AC4 one audit event per command, actor and reason, no reading content | `TestMeterCorrectionConfirmVoidAndRefusals` (audit rows with target `meter_reading`, empty reason code); reason and actor in the history row (`TestMeterCorrectionSupersedesOnceAndReevaluatesNeighbours`) |
| AC5 correcting one reading returns its consistent suspect neighbours to `VALID` | `TestMeterCorrectionSupersedesOnceAndReevaluatesNeighbours` (R4 `VALID`, revision 2, current) |
| Done when: example B end to end through the routes | `TestMeterCorrectionExampleBThroughComposedCompanyHTTP` |
| DRIVER calling the command | composed HTTP 403 `FORBIDDEN` (decision) and 403 on an unassigned asset's history |
| Supervisor of another unit (AUTH-006) | composed HTTP: Sul `UNIT` supervisor 403 on a Norte asset; Norte supervisor decides |
| Another organization's reading | composed HTTP 404; store `NOT_FOUND` with nothing changed |
| Two reviewers correcting the same reading at once | `TestMeterCorrectionConcurrentReviewersOneWins` (both arrival orders queued behind a held root lock; exactly one wins, the other `READING_ALREADY_SUPERSEDED`, one void row) |
| Correction that itself breaks monotonicity | `TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation` (replacement `SUSPECT` `READING_BELOW_PREVIOUS`, reported) |
| Missing reason | composed HTTP 422 `VALIDATION_FAILED`; command shape test |
| Approved parent → amendment | `TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation` (`AMENDMENT_REQUIRED`, revision unchanged) |
| Replay and digest reuse | supersession test (same result, `OPERATION_ID_REUSED`); composed replay `replayed: true` |
| Recalculation event | amendment/recalculation test (one intent, identifiers and revision only; replay adds none) |
| Stale revision, void terminal, opening anchor, foreign reading | `TestMeterCorrectionConfirmVoidAndRefusals` |
| Export state as of `as_of` | `TestReadingListExportsTheStateAsOfTheHistory` |
| Contract | `TestMeterCorrectionContractExamplesAndExactShapes`, inventory/router tests in `cmd/api`, AUDITOR walk |

## Validation — RED → GREEN and gates

Environment as EMP-METER-003 (Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
shared PostGIS `127.0.0.1:32775` through orgtest child databases, throwaway
MinIO `ANPFUEL_VALIDATION_PROJECT=f4-meter003-s3` for the storage-dependent
suites). Logs in `/data/dev/worktrees/f4-logs/meter004-*`.

- Shape first: rules and EDRs, then exported names with stubbed
  `NewMeterDecisionCommand`, `MeterCorrectionService.Decide/History/Suspects`
  and `MeterStore.DecideMeterReading/MeterReadingHistory` (all
  `ErrUnavailable`), the routes, SQL, ports, OpenAPI and inventory.
- RED unit (`go test -json ./internal/modules/fleet/application/ -run TestMeterDecisionCommandShapeAndDigest`)
  → `meter004-unit-red.jsonl`: 1/1 fails (the first valid command is refused).
  RED PostGIS (`go test -json -race -tags=integration ./internal/modules/organization/adapters/ -run '^TestMeterCorrection'`)
  → `meter004-db-red.jsonl`: 5/5 top-level and 7 subtests fail, all on the
  stubbed decision/history (composed HTTP 503 on the suspect list; store
  "source unavailable"; the race helper sees no waiter). A first RED attempt
  hung because the concurrency test called `t.Fatal` inside a goroutine and
  kept its root lock; the test was made failure-safe (errors returned from
  goroutines, deferred release) before the recorded RED. The contract test was
  written with the OpenAPI blocks and passed at once (no contract RED is
  claimed).
- GREEN corrections, each recorded before the fix: the composed decision
  needs `SELECT (org_id, reading_id, review_state) ON org_fuelings` for the
  approved-parent port (granted in the HTTP test and listed in the backend
  README); the race helper counted only sessions blocked directly by the
  holder, while the second decision queues behind the first (now any lock
  waiter other than the holder); the AUDITOR walk needed a delivered-record
  case for the new positive history read.
- GREEN PostGIS focused → `meter004-db-green.jsonl`: 5 top-level / 7
  subtests pass (composed HTTP 22 request assertions). Export
  `./internal/modules/orgexport/adapters/lists` (`-race -tags=integration`)
  → `meter004-export-lists.jsonl`: 8 top-level / 15 subtests pass, including
  `TestReadingListExportsTheStateAsOfTheHistory`.
- Affected race union with MinIO `go test -json -race -tags=integration -count=1 -p 2 -timeout=60m ./internal/modules/organization/adapters ./internal/modules/fleet/... ./internal/modules/fueling/... ./internal/modules/orgsync/... ./internal/modules/orgexport/... ./internal/platform/migrate/... ./internal/platform/enterprisesuite/... ./internal/platform/orgapi/... ./internal/platform/apicontract/... ./cmd/api/...`
  → `meter004-race-union.jsonl`: 23 packages, 855 top-level / 5 962 subtests
  pass, one failure (the AUDITOR walk, missing the history case above;
  organization adapters 1 266.0 s). After the fix
  `^(TestAuditorReadWriteMatrixActualCompanyHTTP|TestMeterCorrection)` →
  `meter004-auditor-rerun.jsonl`: 6 top-level / 95 subtests pass (AUDITOR: 113
  routed normal routes, 25 positive, 88 FORBIDDEN with unchanged private
  state).
- Untagged `go test -count=1 -timeout=45m -p 2 ./...` →
  `meter004-untagged-all.log`: 214 packages ok, 78 without tests, 0 failures.
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean;
  toolchain `gofmt -l ./internal ./cmd ./db` empty; staticcheck (go1.27.1)
  plain on fleet, fueling, orgexport, orgapi, enterprisesuite, apicontract,
  cmd/api and organization, and `-tags=integration` on organization adapters,
  fleet and orgexport: no findings; `sqlc vet`, `sqlc generate`, `sqlc diff`
  clean (fleet and fueling stanzas); OpenAPI `vacuum lint -r
  contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml` 0 errors,
  971 warnings / 616 informs versus 953 / 615 at `2669080c` (the existing
  enterprise route patterns); `git diff --check` clean; `check_plan.py` 348
  tasks / 545 rules / 167 use cases / 145 screens, 0 problems. No Kotlin or
  Android change.
- Secret/PII review of the staged diff (pattern scan and gitleaks): synthetic
  identifiers, labels and reasons only.

## Retained (not part of this task's acceptance)

- The amendment consumer of the PLAT-016 kernel: a decision on a reading of
  an `APPROVED` parent is refused `AMENDMENT_REQUIRED` (EDR-189); no producer
  can approve a fueling yet, so the path is proven with a test port.
- `meter.usage.recalculated` consumers (EMP-METER-006 projection, fueling
  consumption): production publishes nothing; the recalculation is
  identifiable by the operation and the history revisions.
- The organization-wide suspect queue filtered by structural scope, the
  digest notification and the panel screens SCR-METER-02/03/05
  (EMP-METER-007).
- Author edits of a parent record in `DRAFT`/`RETURNED` (BR-METER-015) belong
  to the parent owners; period locks (BR-AMD-004) to the expenses epic;
  replacement, reset and rollover (EMP-METER-005), including corrections of
  the installation opening reading.
- Production grants (backend README).

## Shared files touched

`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md` (section appended),
`docs/enterprise/16-decisions.md` (EDR-187..189 appended),
`docs/enterprise/backlog/ep05-fleet-meters.md` (state and note),
`contracts/openapi/v1.yaml` (3 paths, 7 schemas, 2 error codes),
`contracts/testdata/enterprise/authorization-routes-v1.json` (+3 routes),
`backend/cmd/api/authorization_inventory_test.go` (normal 113),
`backend/cmd/api/enterprise_router_test.go` (+3 paths),
`backend/internal/modules/organization/adapters/auditor_matrix_http_integration_test.go`
(113/25/88, `{reading_id}`), `backend/internal/platform/orgapi/errors.go`,
`backend/README.md`, `backend/internal/modules/organization/adapters/resource_structure.go`.
The authorization-matrix test needed no change: `meter.reading.correct` keeps
its representative; check-manifest unchanged (no new package).
