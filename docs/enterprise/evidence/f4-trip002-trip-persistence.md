# EMP-TRIP-002 — Trip persistence and sync handlers with the one-active-trip-per-asset rule

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 DB task (module TRIP, cut C1, P0, size L) in worktree branch
`codex/enterprise-f4-trip001`, fast-forwarded to the integrated F4 head
`48e9069d` (TRIP-001/017 merged). It persists the EMP-TRIP-001 driver-initiated
trip through the signed intake: one start and one completion per trip, at
most one trip per asset in progress, and the second admissible fact kept for
review instead of dropped. Synthetic data only; no Android, panel, route,
job or deployment change.

Sources read: the EMP-TRIP-002 block and the TRIP-004/005/007/011/014 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3–10
(BR-TRIP-001..004, UC-TRIP-01); 08-state-model §§1, 3, 4; BR-OFFL-004/006/007
and the outcome rows `ASSET_BUSY`, `ASSET_UNAVAILABLE`, `TRIP_ALREADY_CLOSED`,
the offline matrix rows "Start trip"/"Finish trip" and §server concurrency of
[10-offline-sync](../10-offline-sync.md); VAL-01 and VAL-07; the permission
rows `trip.trip.start` (S cells) and `trip.trip.read`; the FUEL-002 precedent
([evidence](f3-fuel002-fueling-persistence.md), EDR-175..177) and its code;
the OFFL-008 held-review mechanism ([evidence](f2-offl008-real-review.md),
migration 000199, `fleet/adapters/meter_review*.go`); FLEET-003 open work
(EDR-144); FLEET-005 snapshot port; the latest usage sources (000215).

IDs used: B-BR-EMP-312..319, BUC-EMP-157..160, EDR-202..206 (all of them).
Migration `000220_organization_trips.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-312..319 and BUC-EMP-157..160 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-002 — Trip persistence and sync handlers with the one-active-trip-
per-asset rule (2026-11-15)"; decisions EDR-202 (operation shapes and the
mapping of EMP-TRIP-001 refusals onto the frozen ACK vocabulary), EDR-203
(storage, guarded completion update, partial unique index, held tables and
the review-source extension), EDR-204 (held facts now, resolution retained to
EMP-TRIP-007), EDR-205 (review case begins as DRAFT; nothing submits) and
EDR-206 (trip-backed open work, active-trip query, `TripAuthority`) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

## Implementation

- Migration `000220_organization_trips.sql` (append-only): forced-RLS
  `org_trips` (start fact, same-organization composite foreign keys to the
  operation, asset, driver and author employee records, author membership,
  enrollment, TRIP_START reading, unit and cost centre; reading-or-reason,
  run-clock pair and snapshot CHECKs; `org_trips_one_active_asset` = UNIQUE
  (org_id, asset_id) WHERE state='IN_PROGRESS'); insert-only
  `org_trip_finishes` (one per trip); insert-only `org_trip_held_starts`
  and `org_trip_held_finishes` tied to OFFL-008 review items. BEFORE triggers
  prove the source operation (type, schema, entity, digest, active enrolled
  author and its employee), refuse a non-ACTIVE asset or a start at/after
  disposal, a reserved trip identity, an inactive or merged driver, the
  FLEET-005 snapshot and the start reporting day/zone/source; the finish
  trigger proves the starting member, IN_PROGRESS, time order, end day in the
  stored zone and a meter of the trip's asset, and its AFTER trigger performs
  the only permitted update (IN_PROGRESS → COMPLETED), guarded by a row
  trigger that refuses any other change. Deferred constraint triggers publish
  a start or finish only with its ACCEPTED ACK and effect (revision 1 / 2)
  and the TRIP_START/TRIP_END reading of the same operation, author,
  enrollment and declaration. Held triggers re-prove the held condition
  (ASSET_BUSY needs a trip in progress, ASSET_UNAVAILABLE a non-ACTIVE or
  disposed asset, TRIP_ALREADY_CLOSED a COMPLETED trip of the same author).
  `org_sync_review_source_valid` keeps its meter branch and adds the two trip
  owners; the asset (000217 body), unit and cost-centre (000215 bodies)
  usage sources are redefined keeping every line and adding the trip keys.
- Application (`trip/application`): the `trip.start`/`trip.finish` codecs
  (exact payloads, METER-001 reasons and values, TRIP-017 run clock, the
  EMP-TRIP-001 `Start`/`Finish` re-proved at parse and at receipt through
  `StartDomain`/`FinishDomain`), ports (`Authority`, `Readings`, `Store`
  including the OFFL-008 `ReviewProof`) and policies (`trip.trip.start`,
  `trip.trip.read`).
- Adapters (`trip/adapters`): sqlc store (`db/queries/trip`), the two
  handlers (authority/target, fleet root and asset locks, final MALFORMED
  checks, held decisions under the locks, `RecordEmbedded`, insert, final
  re-classification; finish re-reads the trip under the locks), the held
  store and proofs, `OpenWork` (FLEET-003) and `ActiveTripsOfAssetAtDBTX`.
- Authority (`organization/adapters/trip_authority.go`): `TripAuthority`
  derived from the fueling authority with `tripHistoryRule` on the
  generalised classifier, the stored author as the record's author, and a
  device-only base admission for a trip not stored here.
- Runtime: `enterprisesuite` registers both owners (NO_ASYNC_FOLLOWUP for
  ACCEPTED/REJECTED, ATOMIC_REVIEW for ACCEPTED_FOR_REVIEW) and wires
  `trippg.OpenWork{}` instead of `NoOpenWork`; OpenAPI declares the two
  owners and `EnterpriseTripStartPayload`/`EnterpriseTripFinishPayload`
  (`apicontract` `TestTripOwnersAreExplicitAtomicReviewOwnersWithExactPayloads`,
  meter owner count 2 → 4); `orgtest.GrantMeterUsage` grants the trip
  usage/open-work columns; `scripts/check-manifest.txt` lists the two new
  packages. No route, authorization-matrix or inventory change.

## RED

The handlers and store were drafted before the integration tests (an honest
deviation from test-first order). RED was therefore obtained by freezing the
tests and the migration (shape) and running them against stub bodies: both
handlers' `Admit`/`Apply`/`Confirm` returning `ErrUnavailable`, `OpenWork`
and the active-trip query answering nothing. Log
`/data/dev/worktrees/f4-logs/trip002-go-red.jsonl` (`-tags=integration
-race -p 2 -run '^TestTrip'`, 86.8 s): 11 top-level tests, 9 failing; the two
passing are the pure history-rule test (real classifier, not stubbed) and the
populated upgrade test (the migration is shape); 10 history-rule subtests
pass, both race subtests fail. A first RED attempt hung because the race
barrier waited inside the handler that the stub never reached; the barrier
now has a 30 s timeout. No expectation changed.

## GREEN and REFACTOR

The first GREEN run failed for fixture reasons: the fixture declared every
event four hours in the past, before the ADMIN member's role existed, so the
real classifier held them `PERMISSION_CHANGED` (correct behaviour; the
fixture now declares at the database's now, like FUEL-002), and the race
barrier placed in the handler could not create contention because the intake
serializes per organization at source confirmation. The race now holds the
winner inside its intake transaction (after admission), starts the loser,
observes it waiting on a lock in `pg_stat_activity`, and only then lets the
winner proceed — in both orders. Log
`/data/dev/worktrees/f4-logs/trip002-go-green.jsonl`: 11/11 top-level and
21/21 subtests pass (21.1 s, race). staticcheck found two dead assignments in
the codec test (SA4006), fixed.

| Requirement | Test(s) |
|---|---|
| AC: concurrent starts → one active trip, the second held for review (VAL-07), both orders, two devices | `TestTripIntakeConcurrentStartsKeepOneActiveTripAndHoldTheOther/{first-wins,second-wins}` (lock wait observed; one IN_PROGRESS; held start with its reading values, review item and v1 ACK proof; no reading or active use; byte-identical replays of both ACKs) |
| AC: matching replay returns one acknowledgement; digest reuse | `TestTripIntakeStartsAndFinishesTheVAL01TripAndReplays` (2+2+2 resends, `OPERATION_ID_REUSED`, counts unchanged) ; `TestTripIntakeConcurrentDuplicatesLeaveOneTrip` (12-way) |
| VAL-01 start/finish, 290 000 m, exact stored fact, unit snapshot and start day, TRIP_START/TRIP_END, run clock, completion | first test (SQL proof of every column, reading difference 290 000) |
| Foreign asset (not found) | `TestTripIntakeRefusalsAndHoldsAtReceipt/foreign-asset-not-found` |
| Duplicate active trip | `…/busy-asset-held` (ASSET_BUSY) and the race test |
| Uncommitted receipt | `TestTripIntakeRollsBackTheReadingAndTheTripTogether` (failure after the real reading insert: no claim, receipt, reading or trip; the same operation then commits) and the deferred publication triggers |
| Silently dropped conflict | held starts and finishes with review items and replayable ACKs (race, busy, `TestTripIntakeKeepsASecondFinishAsTripAlreadyClosed`) |
| Finish before start | `…/finish-before-start-malformed` |
| Finish of an unknown or held trip | `…/finish-of-unknown-trip-pending`, `…/busy-asset-held` (DEPENDENCY_PENDING, nothing stored) |
| Identity reuse, reading and reason both, unknown meter | `…/trip-identity-reused-malformed`, `…/reading-and-reason-malformed`, `…/unknown-meter-pending` |
| Authority: SELF never assigned, another member's finish, changed right | `…/self-driver-never-assigned-forbidden`, `…/another-member-finish-forbidden`, `TestTripIntakeHoldsAStartWhoseRightChanged`; pure `TestTripHistoryRuleCoversOwnTripsOnAssignedAssets` (10 cases) |
| Open work (FLEET-003) and unavailable asset | `TestTripOpenWorkAndUnavailableAsset` (deactivation refused with `TRIP` while in progress, allowed after completion; a start on the INACTIVE asset held ASSET_UNAVAILABLE) |
| Active trip at an instant | first test and race test (`ActiveTripsOfAssetAtDBTX`) |
| Forced RLS, guarded update, insert-only, partial unique index | `TestTripSchemaIsForcedRLSGuardedAndInsertOnly` |
| Populated upgrade 000215 → 000220 | `TestTripPopulatedUpgradeFailureRecoveryAndPreviousImage` (injected failure leaves schema/functions/data unchanged; recovery once; no-op replay; previous image compatible; ordinary forced-RLS migrator owns the four tables; usage answers unchanged; meter review branch kept) |
| Codec and contract | `trip/application` `TestParse*`, `TestFinishDomainAppliesTheTripRulesAtReceipt`; `trip/adapters` unit tests; apicontract test |

## Validation

Commit 1 (this task's source) — Go from `backend`, toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f4-go-build`, real PostGIS through `orgtest`:

```sh
gofmt -l ./internal/modules/trip ./internal/modules/organization ./internal/platform/{enterprisesuite,orgtest,apicontract}   # empty
go build ./... && go vet ./... && go vet -tags=integration ./...                               # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/trip/... ./internal/modules/organization/... ./internal/platform/{enterprisesuite,orgtest,apicontract}/...   # clean
sqlc vet && sqlc generate                                                                      # no diff beyond the new db/queries/trip
go test -race -count=1 -cover ./internal/modules/trip/...                                      # domain 100.0%, application 86.2%, adapters unit 7.7% (integration covers the rest)
go test -json -race -tags=integration -timeout=60m -p 2 -count=1 ./internal/modules/{organization,fleet,orgsync,fueling,trip,membership,orgaccess}/... ./internal/platform/{migrate,enterprisesuite,apicontract,orgtest}/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
python3 docs/enterprise/tools/check_plan.py   # problems 0
```

The union (log `trip002-db-union.jsonl`, organization/adapters 1294 s)
passed 29 packages; organization/adapters failed 12 top-level tests: six
need the disposable object store (`ANPFUEL_TEST_STORAGE_ENDPOINT`, untouched
document/export/auditor HTTP surfaces) and six were the FUEL-002 fixture-copy
trap — `installSyntheticCostCentreSource` and the unit equivalent copy the
usage-source allow-list and lacked the new `org_trips_*_fkey` lines. Both
copies now list them and the six cost-centre/unit tests pass (45.6 s, race).
The storage-dependent tests run in the merged-tree union below with a
throwaway MinIO. The untagged suite passes (exit 0, 217 packages ok, log
`trip002-go-untagged-all.log`).

The FLEET-007 availability wiring and the merged-tree validation are the
second TRIP-002 commit (section "FLEET-007 wiring" below).

## FLEET-007 wiring (second TRIP-002 commit)

After the first commit (`f12558a2`) the branch merged `codex/enterprise-f4`
(`5a206693`: `org-permissions-v5`, EMP-FLEET-007 migration 000217 with
reservations, blocks, the derived availability, `NoActiveUse`,
`StartDecisionDBTX`) in a normal merge commit (`c196ac5a`; the contract and
runtime conflicts kept both sides). The second commit then:

1. rebuilds the 000220 asset usage source from the 000217 body, so it keeps
   the reservation and block keys beside the trip keys (the upgrade test now
   asserts both);
2. consults the FLEET-007 `StartDecisionDBTX` in the start intake after the
   history and lifecycle checks: a block, another employee's confirmed
   reservation, maintenance, a missing assignment or a trip holding the
   declared instant holds the start `ASSET_UNAVAILABLE` / `ASSET_BUSY`; the
   held-start trigger now re-proves only `ASSET_BUSY` (a trip in progress or
   one holding the declared instant) and leaves the other codes to their
   owning ports in the same transaction;
3. replaces `NoActiveUse` with the trip-backed `ActiveUse` (stored trips
   holding the asset at the instant) in the runtime, so availability reads
   `IN_USE` and lists only `MAINTENANCE_HOLD` as missing;
4. found no `org-permissions-v4` pin in the trip code or tests (none added).

The trip fixture now assigns the ADMIN member to the vehicle (FLEET-007
admits a start only for an assigned or the reserved driver). New PostGIS
tests: `TestTripIntakeHoldsAStartInsideAnotherEmployeesReservation` (FLEET-007
AC4: an assigned driver outside the confirmed reservation is held
ASSET_UNAVAILABLE, the reserved driver is accepted, availability reads
IN_USE from the trip source) and `TestTripIntakeBlockDuringATripAppliesToTheNextStart`
(AC5: a SAFETY block during the trip does not refuse its finish and holds the
next start). The first run of those two failed RETRY/UNAVAILABLE because the
held-start trigger still required a non-ACTIVE asset for ASSET_UNAVAILABLE;
the trigger was corrected (the migration is not yet applied anywhere), no
expectation changed. Trip tests: 13/13 top-level, 21/21 subtests (34.5 s,
race), log `trip002-go-green-wiring.jsonl`.

Merged-tree validation (commit 2, same commands plus a throwaway MinIO
project `f4-trip002-s3` from `infra/compose.validation.yml`, removed with
`down -v` afterwards): gofmt, build, vet (plain and integration), staticcheck
(plain and integration on trip, organization, fleet, enterprisesuite, orgtest,
apicontract) and `sqlc vet`/`generate` are clean (log
`trip002-merged-static.log`). The race/integration union over organization,
fleet, orgsync, fueling, trip, membership, orgaccess, document, migrate,
enterprisesuite, apicontract and orgtest (log
`trip002-merged-db-union.jsonl`; organization/adapters 1209 s) passed 34
packages and failed two organization/adapters tests that the wiring itself
caused: the runtime availability read now queries trips, so
`orgtest.GrantMeterUsage` also grants the trip identity, instant and finish
columns, and the FLEET-007 HTTP test now expects only `MAINTENANCE_HOLD`
among the missing sources (OpenAPI description and example updated). Both
tests, the auditor export gates, every `TestTrip*` and `TestFleetAvailability*`
then pass (51.4 s, race, MinIO), and apicontract passes. The untagged suite
passes (exit 0, 217 packages ok, `trip002-merged-untagged-all.log`);
`check_plan.py` reports 0 problems.

## FLEET-011 integration (EDR-219)

Third TRIP-002 change, made on `codex/enterprise-f4-trip003` from the F4 head
`6a843325` (TRIP-002 and EMP-FLEET-011 both merged), closing the items both
evidence files retained. No migration, route, OpenAPI or Kotlin change.

1. **AC4 — an implement is never the main asset.** `TripStartFacts` now
   also reads the asset kind under the root and asset locks; the start
   intake applies the pure `fleet.MainAssetAllowed` next to the `DRAFT`
   check and before every held condition: an `IMPLEMENT` start is a final
   `REJECTED` `MALFORMED` with nothing stored (no trip, reading, held start,
   review item or effect), replayed byte for byte.
2. **AC2 — implements attached at the start.** `trippg.StartImplements`
   (`ImplementsAtStartDBTX(trip)`) reads the stored trip's asset and declared
   start instant and lists the implements through Fleet's
   `AttachedImplementsDBTX`. No snapshot is stored: attachment periods never
   change toward the past (no retroactive attach; a detach never ends before
   its transaction instant), so the set at a past start is stable and a copy
   would duplicate the Fleet-owned fact. A trip not stored in the
   organization answers `ErrNotFound`.
3. **AC3 — implement in use through a real trip.** No code: the runtime
   active-use source is already the trip-backed `ActiveUse` (EDR-206) and the
   availability read already follows an implement's attachment (EDR-212).

Tests (PostGIS, `-race -tags=integration`, written before the change):
`TestTripIntakeRefusesAnImplementAsTheMainAsset` (the fixture's ACTIVE
IMPLEMENT with an observed odometer, assigned to the member: with a reading
and with a missing reason, both `MALFORMED`, replays, no trip on the
implement; the same member's vehicle start is then accepted) and
`TestTripStartListsAttachedImplementsAndCarriesThemInUse` (two implements
attached before the start are listed in identifier order; one detached before
the start, one attached only from an hour after it and one never attached are
not; an unknown trip is `ErrNotFound`; while the trip is IN_PROGRESS each
attached implement reads `IN_USE` with the trip as cause and the vehicle as
`powered_asset_id`, the vehicle its own cause without `Via`, the others
`AVAILABLE`; after `trip.finish` the attached implements read `AVAILABLE`
and the start listing is unchanged). The trip fixture now grants the
FLEET-011 attachment tables (the availability facts of an implement read
them).

RED (`/data/dev/worktrees/f4-logs/wiring-red.jsonl`, against the unwired
code with a zero-value `StartImplements` stub): 2/2 top-level and 2/2
subtests failed for the right reasons — the implement start was `ACCEPTED`
(and the second subtest held `ASSET_BUSY` behind it, one trip stored on the
implement) and the listing was empty (three listing assertions). The AC3
assertions, ordered before the listing checks, passed against the unwired
code, which is the evidence that AC3 needed no wiring. A first RED attempt
failed for the wrong reason (`RETRY`/`UNAVAILABLE`: the fixture lacked the
attachment `SELECT` grant that the FLEET-007 facts of an implement need); the
fixture grant was added and the RED rerun, no expectation changed.

GREEN (`wiring-green.jsonl`): every `TestTrip*`, `TestFleetImplement*` and
`TestFleetAttachment*` PostGIS test, 21/21 top-level and 23/23 subtests
(race). Unit tests of trip, fleet and enterprisesuite pass; `sqlc
generate`/`vet`/`diff` (the regenerated query adds `asset_kind`), toolchain
gofmt, `go vet` (both tags) and staticcheck (plain and integration on trip,
`db/queries/trip` and organization adapters) are clean. Race/integration
union with PostGIS and the coordinator MinIO over organization adapters,
trip, fleet, orgsync, enterprisesuite, apicontract, migrate, orgtest, orgapi
and cmd/api (`wiring-union.jsonl`, 21 min, organization adapters 1306 s): 18
packages pass (orgtest has no tests), 889 top-level / 6296 subtests, 0
failures, 0 skips. `check_plan.py` reports 0 problems.

## Retained obligations

- EMP-TRIP-007: resolution of held starts and finishes (accept with this or
  another asset, refuse with a reason), their company queue listing and the
  author own-result read; until then a trip review item is invisible in the
  meter-typed queue and any decision on it fails closed. Reserving a held
  start's reading identity against other operations also belongs there.
- A finish whose rights changed after its declaration stores nothing and
  stays `RETRY`/`UNAVAILABLE` (no held-finish owner for PERMISSION_CHANGED).
- EMP-TRIP-004 appends SUBMIT and the review decisions (EDR-205);
  EMP-TRIP-005 is the Android producer; EMP-TRIP-006 the panel reads;
  EMP-TRIP-011/012/014 pause, handover and supervisor closure; the
  maintenance epic replaces `NoMaintenanceHolds`.
- EMP-FLEET-011 integration (closed by EDR-219, section above): the
  start applies `MainAssetAllowed` and the implements attached at a start are
  listed through `AttachedImplementsDBTX` (derived at read time, no
  snapshot); the attached implement is `IN_USE` through a real trip. Still
  retained: a consumer of the start listing (EMP-TRIP-006) and an optional
  database guard against an implement trip.
- No route, panel read, Android or job change; no C1, M4 or F4 acceptance.
