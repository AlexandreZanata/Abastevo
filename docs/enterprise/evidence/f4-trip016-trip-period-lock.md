# EMP-TRIP-016 — Period lock and authorised reopening for trips

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 task (module TRIP, P1, type API, size S) in worktree branch
`codex/enterprise-f4-trip016`, opened from the F4 integration head
`7daf0010` (TRIP-001/002/003/004/017, FLEET-007/011, METER-003/004,
OFFL-006/009/012, CHECK-001, catalog v5; migrations through 000226). It is
the first real consumer of the APPR-006 period gate: a trip review decision
is admitted only while the period of the trip's stored start reporting day
is writable, field intake stays unblocked, and the database refuses a
decision in a locked month. Synthetic data only; no Android, panel, job,
deployment, shared vector or route change.

Sources read: the EMP-TRIP-016 block and the TRIP-004/015/017 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3–8, §10;
BR-AMD-001..006, BR-TIME-003/004, BR-REV-007, BR-AUD-001
([04-business-rules](../04-business-rules.md)); JRN-09
([05-journeys](../05-journeys.md)); REQ-07.5
([17-traceability](../17-traceability.md)); the ACK table row "Reporting day
in a locked period" and BR-OFFL-006 ([10-offline-sync](../10-offline-sync.md));
the Period row of [08-state-model](../08-state-model.md); the
`fin.period.close`/`fin.period.reopen` and `trip.trip.review` matrix cells
([07-permissions](../07-permissions.md)); VAL-19 and VAL-26
([15-validation](../15-validation.md)); the APPR-006 evidence
([f3-appr006-period-lock](f3-appr006-period-lock.md): `PeriodGate`,
`PeriodFact.Writable/Amendment`, the root-lock protocol and its race
barriers), the PLAT-016 kernel evidence
([f3-plat016-amendment-kernel](f3-plat016-amendment-kernel.md): period
checked last, adjustment only from `LOCKED` into a later writable day),
the TRIP-002/003/004 evidence and EDR-149, EDR-177, EDR-182, EDR-220..223;
the B-BR-EMP-266 precedent (fueling intake never consults the gate).

IDs used: B-BR-EMP-359..362, BUC-EMP-181..182, EDR-232..233 (all of them).
Migration `000228_organization_trip_period_lock.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-359..362 and BUC-EMP-181..182 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-016 — Period lock and authorised reopening for trips
(2026-11-15)"; decisions EDR-232 (the gate keyed on the stored start
reporting day, its place after the lifecycle checks, ungated replays, reads
and field intake, no adjustment form) and EDR-233 (the 000228 database
guard) are in [16-decisions](../16-decisions.md). They are the recommended
routine decisions, taken unattended.

Design answers the task asked to decide:

- **Key and zone.** The trip's stored `reporting_day` (attributed at the
  start receipt in the stored unit or organization zone by the 000220
  trigger, EDR-182) is passed to `PeriodGate.StateForDayTx`. The gate is
  never asked to attribute an instant (`StateForOccurrenceTx` would read the
  unit's *current* zone), and the device entry zone/offset, the UTC day, the
  finish day and the decision's receipt day play no part.
- **Refuse or adjust.** Refuse. A trip carries no amount posted to a
  period, so BR-AMD-004's adjustment branch has nothing to post; a decision
  in a `LOCKED` month is `PERIOD_LOCKED` (409) and an authorised reopening
  (`fin.period.reopen`, fresh proof, reason) is the only path. The orgamend
  `ADJUST` intent stays unused by trips until an owner posts trip amounts.
- **Late field facts.** Accepted as facts, never held `PERIOD_LOCKED`
  (B-BR-EMP-360): a held intake item needs a resolution owner (EDR-177) and a
  held start is never a trip (it would hide active use). The submitted trip
  waits in the review queue, which is its review; its decision needs the
  reopening. `trip.start`, `trip.finish`, `trip.event.record` and
  `trip.submit` code is untouched.
- **Order.** Authority (root `FOR UPDATE`), trip row lock, replay lookup,
  APPR-001 lifecycle (`SELF_APPROVAL`, `CONFLICT`, `VALIDATION_FAILED`
  keep their codes), then the period, then the entries and audit event —
  the orgamend order (period last). Replays of decisions recorded before a
  lock return their stored result; reads and the queue are not gated.

## Implementation

- `trip/application/period.go`: `ErrPeriodLocked` and the pure
  `DecisionPeriod(reportingDay, fact)` (the fact must be the gate's valid
  answer for exactly the stored day; `PeriodFact.Writable` decides: `OPEN`,
  or `REOPENED` with its stored reasoned reopening of the head revision).
- `trip/adapters/review_store.go`: `ReviewStore.Periods`
  (`reviewapp.PeriodGate`, required by `ready`); `DecideTripReview` reads
  the gate inside its transaction after `app.Decide` and before
  `RecordChangeTx`.
- `trip/adapters/http/review.go`: `ErrPeriodLocked` → `PERIOD_LOCKED`.
- `enterprisesuite/routes.go`: the trip review store is composed with
  `reviewpg.PeriodGate{}`.
- Migration `000228`: `CREATE OR REPLACE FUNCTION
  org_trip_review_entry_source()` with the complete 000226 body plus one
  clause after every existing check: an `OPEN`/`APPROVE`/`RETURN`/`REJECT`
  entry whose trip's `date_trunc('month', reporting_day)` head in
  `org_finance_periods` is `LOCKED` raises `55000` "trip reporting period
  locked". The trigger already holds the organization root `FOR UPDATE`
  (as every period command does), so the head read is current. No table,
  column, index or usage source changes; `sqlc.yaml` is unchanged (no query
  or table changed).
- OpenAPI: the decide description names `PERIOD_LOCKED`; no schema, path or
  inventory change (route counts stay 134 normal, 24/60/25; matrix 240
  cells; AUDITOR walk 134/30/104).

Shared trip files touched (another agent works on TRIP-008 and the intake):
`trip/adapters/review_store.go` (one field, `ready`, one gate call),
`trip/adapters/http/review.go` (one mapping pair),
`trip/adapters/review_test.go` and `http/review_test.go` (tests),
`organization/adapters/trip_review_integration_test.go` (fixture grant
`SELECT ON org_finance_periods,org_finance_period_transitions` and the
`Periods` port). None of `handlers.go`, `intake.go`, `store.go`,
`events.go`, `submit.go`, `trips.go`, `codec.go` or the trip SQL queries.

## RED

Shape first: rules and EDRs, `ErrPeriodLocked`/`DecisionPeriod` stubbed to
an unexported not-implemented error, the store field and gate call, the
HTTP mapping, the suite composition, the migration as a shape (the 000226
body replaced unchanged, without the period clause, kept at
`f4-logs/trip016-scripts/000228-red-shape.sql`), then the tests.

- Unit (`go test -json -race -count=1 ./internal/modules/trip/...`), log
  `/data/dev/worktrees/f4-logs/trip016-go-red-unit.jsonl`: the new
  `TestDecisionPeriodAdmitsOnlyAWritablePeriodOfTheStoredDay` fails (12
  cases); `TestReviewStoreNeedsThePeriodGate` and the extended mapping test
  pass at once because they check shape code only; 41 top-level pass in all.
- PostGIS stage 1 (all stubs; `-run '^TestTripPeriod'`, 35 s), log
  `trip016-go-red-integration.jsonl`: 5/5 top-level and 2/2 subtests fail,
  all behavioural (`period decision not implemented`; the composed route
  answers 503 instead of 409).
- PostGIS stage 2 (Go implemented, migration still the shape), log
  `trip016-go-red-integration-schema.jsonl`: the gate, race and HTTP tests
  pass (3/5); the database-guard test fails because a direct opening in the
  locked month passes the BEFORE trigger and fails only at commit
  (`23514 trip review opening without decision` instead of `55000 trip
  reporting period locked`) and the upgrade test finds no guard installed
  (2/5 fail). That is the real gap 000228 closes.

## GREEN and REFACTOR

- `DecisionPeriod` implemented; the final migration
  (`f4-logs/trip016-scripts/000228-final.sql`) installed.
- Unit: trip packages green under `-race` (42 top-level / 413 named).
- PostGIS focused (`-run '^(TestTripPeriod|TestTripReview|TestTripSubmit)'`),
  log `trip016-go-green-integration-1.jsonl`: 12/12 top-level and 4
  subtests pass — the 5 new tests and the 7 TRIP-004 tests (whose fixture
  now composes the gate).
- REFACTOR: toolchain gofmt reordered two import blocks; staticcheck clean.

## Named negatives and acceptance → tests

| Requirement | Test(s) (real PostGIS unless noted) |
|---|---|
| AC: a locked period keeps its approved facts | `TestTripPeriodLockGatesDecisionsOnTheStoredStartDay` (an approval recorded before the October lock stays `APPROVE:APPROVED:1` with one audit event, and its replay answers `replayed=true` after the lock); race order 1 (the approval committed before the lock is kept); `TestTripPeriodDatabaseRefusesLockedDecisionsAndRewrites` (rewrites refused, rows unchanged) |
| AC: authorised reopening records a reason and retains history | gate test: finance reopens October with a reason, the decision is admitted, the re-lock keeps it; the period history is exactly `LOCK:2`, `REOPEN:3:<reason>`, `RELOCK:4` and the `RETURN` entry lies between the reopening and the re-lock; HTTP test (409 then 200 after the reopening) |
| Wrong reporting zone (stored zone, never the client clock) | gate test: boundary trips whose start `2026-11-01T02:30Z` is November in UTC and on the device (`Asia/Tokyo`) and whose receipt is later, but whose stored day in `America/Sao_Paulo` is `2026-10-31`: locking November does not stop their approval; locking October refuses every decision; `DecisionPeriod` unit cases refuse a fact of another day or month (fail closed) |
| Stale authority | gate test: a decision context read during the reopening; the re-lock withdraws it; the decision then received is `PERIOD_LOCKED` with nothing written; unit cases refuse a `REOPENED` fact without its reopening, of an older revision, or recorded after the check |
| Lock race, both orders with barriers | `TestTripPeriodLockRacesWithTheDecisionInBothOrders`: (1) the real gate pauses the decision after its period read while it holds the root; the FINANCE lock is observed waiting on that backend (`pg_blocking_pids`), then commits after the approval, which is kept; (2) a re-lock holds the root (stopped on the head table by a `SHARE` lock); the decision is observed waiting on the command's backend, then sees `LOCKED` revision 4 and writes nothing |
| In-place rewrite refused by the database | DB test: direct `OPEN` insert in the locked month `55000 trip reporting period locked`; after a reopening the period clause admits it and the publication guard still refuses it at commit; `UPDATE org_trips` (moving the reporting day), `UPDATE`/`DELETE`/`TRUNCATE org_trip_review_entries`, `DELETE org_trips`, `UPDATE org_finance_period_transitions`, `DELETE org_finance_periods` refused `55000`, a forged head `23514`; trip, history and period rows byte-identical |
| Lifecycle refusals keep their codes | gate test: a stale revision in a locked month is `CONFLICT` |
| Field intake not gated (B-BR-EMP-360) | gate test: with October and the current month locked, a new trip is started, finished and submitted (ACCEPTED) and joins the queue; trips of locked months stay in the queue and their decision context is readable |
| Composition | `TestTripPeriodLockThroughComposedCompanyHTTP` (actual `enterprisesuite`: 409 `PERIOD_LOCKED`, no history; after the reopening 200 `APPROVED`); unit `TestReviewStoreNeedsThePeriodGate`; HTTP mapping unit test |
| Populated upgrade from the 000226 head | `TestTripPeriodPopulatedUpgradeFailureRecoveryAndPreviousImage` (approved and submitted trips, a locked month; injected failure keeps the 226 body and every row; ordinary NOSUPERUSER migrator recovery applies exactly `000228`; no-op replay; previous image compatible; then the gate and the database both refuse the locked month, and a reopening admits the decision) |

## Validation

Toolchain Go 1.27.4 from `backend/` (`/data/dev/worktrees/f4-logs/trip016-env.sh`:
`GOCACHE=/data/dev/cache/f4-go-build`, the shared PostGIS through `orgtest`
isolated child databases, the coordinator MinIO `127.0.0.1:32819`,
`validation-private`):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/trip/... ./internal/modules/review/... ./internal/platform/enterprisesuite/... ./cmd/api/...   # clean
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/trip/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite/...   # clean
sqlc vet && sqlc diff                                                 # clean
go test -json -race -tags=integration -count=1 -p 2 -timeout=90m ./internal/modules/organization/adapters ./internal/modules/{trip,review,orgamend,fleet,orgsync}/... ./internal/platform/{enterprisesuite,apicontract,migrate,orgtest,orgapi}/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
python3 docs/enterprise/tools/check_plan.py                           # repo root
git diff --check
```

- Race/integration union, log `trip016-db-union.jsonl` (21 min 18 s): 26
  packages; 24 pass, `orgtest` has no tests, and `organization/adapters`
  was terminated by an external SIGTERM ("signal: terminated") inside the
  unrelated `TestRedemptionIssuerRightsAndOrganizationInvalidateCurrentPreview/managed`
  after 388 of its tests had passed (no assertion failure, no race report).
  The 25 other packages: 468 top-level / 3098 named, 0 failures, 0 skips.
- `organization/adapters` rerun alone with the same flags, log
  `trip016-db-union-orgadapters-rerun.jsonl` (1394 s): 507 top-level / 4139
  named, 0 failures, 0 skips — every `TestTripPeriod*`, `TestTripReview*`,
  `TestTripSubmit*` and `TestPeriod*` test, the authorization matrix at 240
  cells (93 allowed / 147 denied) and the AUDITOR walk at 134 / 30 / 104
  with the composed suite now carrying the period gate. Union total: 975
  top-level / 7237 named, 0 failures.
- Untagged whole module (`GOFLAGS=-p=2 go test -count=1 ./...`), log
  `trip016-untagged-all.log`: exit 0, 222 packages ok, 80 without tests
  (4 min 11 s).
- Static gates (`trip016-staticcheck.log`, empty): toolchain gofmt, vet
  plain and integration, staticcheck plain and integration, `sqlc vet` and
  `sqlc diff` all clean.
- OpenAPI: `vacuum lint` 0 errors, 1018 warnings / 660 informs — identical
  to the base (`trip016-vacuum.log`).
- `check_plan.py`: 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean; the secret keyword scan of the diff
  has no hit and `gitleaks stdin --redact` finds no leaks. Synthetic data
  only.
- No Kotlin, Android, shared vector, job or deployment change, so no Gradle
  run applies; no emulator or device was used.

## Retained obligations

- EMP-TRIP-015: amendments of approved trips go through orgamend with this
  gate (`AMEND`/`VOID` `PERIOD_LOCKED`, approval of an amendment gated
  again); resubmission after `RETURN` and late/duplicate facts.
- An adjustment form for trips (BR-AMD-004 second branch) waits for an
  owner that posts trip amounts or indicators to periods (KPI/budget).
- The decision context and queue do not show the period state; the panel
  (EMP-TRIP-006) may read it through the period routes.
- Unit, cost-centre or team scoped periods (EMP-AUTH-006 / finance policy)
  remain APPR-006 obligations; the guard keys only the organization month.
- Production runtime grants (not provisioned here): `SELECT ON
  org_finance_periods, org_finance_period_transitions` (and the existing
  root row-lock privilege) for the trip review decision; proposed README
  text is this paragraph.
- No C1, M2, M4 or F4 acceptance, deployment or real data is claimed.
