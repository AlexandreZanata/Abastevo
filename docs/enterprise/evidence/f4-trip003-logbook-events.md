# EMP-TRIP-003 — Logbook events: the typed timeline

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PENDING (merged-tree union and untagged suite)
Integration: INTEGRATION_PENDING

Bounded F2/C1 DB task (module TRIP, P0, size M) on
`codex/enterprise-f4-trip003`, built on the F4 head `6a843325` after the
EMP-FLEET-011 × EMP-TRIP-002 integration commit (`6d62c502`, EDR-219). It
appends typed logbook facts to a stored driver-initiated trip through a new
signed operation. Synthetic data only; no route, panel, Android, job or
deployment change.

Sources read: the EMP-TRIP-003 block and the TRIP-004/005/006/010/011/012/
014/015 blocks ([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md)
§§3–10 (BR-TRIP-001..004, UC-TRIP-01); REQ-07.1/07.3/07.7 in
[17-traceability](../17-traceability.md); the glossary entries "Logbook" and
"Logbook event"; [12-ux-app](../12-ux-app.md) §4.8 (the kind grid: stop,
fueling, toll, parking, incident, handover, note); BR-OFFL-001/004/006/007,
the intake decision table and the offline matrix row "Logbook events, stops,
pause, resume — trip not closed by someone else" of
[10-offline-sync](../10-offline-sync.md); BR-TIME-001/002; EDR-028 and LV-05;
VAL-01, VAL-23 and VAL-26; BR-CHECK-014 (incident events) and BR-ROUTE-008/
013/019 (actual stops, arrival events, quick actions); the TRIP-001, TRIP-017
and TRIP-002 evidence and code (handlers, store, migration 000220, held
facts, the OFFL-008 review hook).

IDs used: B-BR-EMP-320..324, BUC-EMP-161..163, EDR-207..209 (all of them).
Migration `000225_organization_trip_events.sql` (append-only; `000221` stays
unused, `000222`/`000224` already exist).

## Rules frozen before behaviour

Rules B-BR-EMP-320..324 and BUC-EMP-161..163 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-003 — Logbook events: the typed timeline (2026-11-15)"; decisions
EDR-207 (operation, closed vocabulary, outcome mapping), EDR-208 (storage,
guards, no usage-source change, no event clock or day) and EDR-209 (no read
route, shared timeline order and Kotlin mirror, follow-up plan) are in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended:

- **Vocabulary.** Supported: `NOTE` (text required), `STOP`, `ARRIVAL`,
  `DEPARTURE` (text optional) — the simple facts of the glossary and UX grid.
  Reserved (recognised, `RETRY`/`UNAVAILABLE`, nothing stored): `FUELING`,
  `TOLL`, `PARKING` (fueling and expense links: EMP-FUEL-011, EMP-EXP-004,
  EMP-TRIP-020), `INCIDENT`, `DAMAGE` (EMP-CHECK-003), `HANDOVER`
  (EMP-TRIP-012), `PAUSE`, `RESUME`, `CANCEL` (EMP-TRIP-011). Planned stops and
  legs (EMP-TRIP-010) would link an `ARRIVAL`/`DEPARTURE` in a later schema.
  Everything else (`START`, `END`, `FINISH`, lowercase, padded, empty) is
  `MALFORMED`: start and finish are their own operations.
- **Time against the trip.** Before the start instant: `MALFORMED` (EDR-202
  finish-before-start precedent). While IN_PROGRESS: any later instant. On a
  COMPLETED trip: at or before the finish instant accepted (a late delivery
  of a fact recorded during the trip; the trip stays COMPLETED); after it,
  held `ACCEPTED_FOR_REVIEW` `TRIP_ALREADY_CLOSED` (the offline-matrix check,
  like TRIP-002's second finish). Only the starting member appends (another
  member `FORBIDDEN`).
- **Another organization's trip.** Forced RLS makes it indistinguishable from
  an undelivered or held trip, so it is "not found" in the organization and
  answered `RETRY` `DEPENDENCY_PENDING` with nothing stored or revealed
  (EDR-202 finish precedent; a `NOT_FOUND` refusal would permanently refuse
  reordered children, BR-OFFL-007).
- **No read route** (EDR-175 precedent): the timeline is the store read
  `EventStore.TimelineTx` for EMP-TRIP-006; the app (EMP-TRIP-005) orders the
  same way through the Kotlin mirror.

## Implementation

- Domain `trip/domain/logbook.go` (pure): `LogbookType` with
  `ParseLogbookType`, `Supported`, `RequiresText`, `LogbookTypes`;
  `ValidLogbookText` (the METER-001 reason profile, equal to SQL
  `org_asset_text_valid(…,280,true)`); `LogbookRef`; `Trip.Log` (scope and
  trip, identity, author, type, text, declaration, starting member, time
  window; the trip is never changed) with `ErrUnsupportedEvent`,
  `ErrEventBeforeStart`, `ErrEventAfterFinish`; `TimelineItem`,
  `CompareTimeline` and `SortTimeline` (original instant; equal instants
  START, events by identity, FINISH; receipt is never a key).
- Kotlin mirror `domain/.../trip/TripLogbook.kt`: `TripLogbookType`,
  `TripLogbookRef`, `TripLogbookInput`, `Trip.log`, `TripLogbookFailure`
  (a separate vocabulary so the trip-v1 failure set is unchanged),
  `TripTimeline`.
- Shared oracle `contracts/testdata/enterprise/trip/logbook-v1.psv` (103
  cases: TYPE 27, RECORD 66, ORDER 10; SHA-256
  `051b1a3a6947d8b7b620be5316ca6747b6f4fed86cde3ce7f7c9eb19171ef614`),
  documented in the trip testdata README; both consumers assert hash, counts,
  unique IDs, allowed arguments, every error code and every vocabulary token,
  that recording never changes the trip and that the order does not depend on
  the input order.
- Application `trip/application/events.go`: `trip.event.record` /
  `trip_event`, exact payload `trip_id`, `type`, `text`; `ParseEvent`
  (`MALFORMED`: envelope, keys, identities, declaration, unknown type, text;
  `ErrUnsupported`: reserved types, soft references, attachments; at most one
  dependency); `StoredDomain` rebuilds the stored trip and, when completed, its
  finish through `FinishDomain` (the finish reading kind and value are now
  read with the trip; `startedDomain` was extracted from `FinishDomain`
  without behaviour change); `EventDomain` applies `Trip.Log` at receipt;
  `EventStore` port.
- Adapters `trip/adapters/events.go`: the owner (authority and target through
  the TRIP-002 `TripAuthority`, history classification on the trip's asset at
  the event instant, root and asset locks, re-read under the locks, identity
  facts, domain at receipt, held event, insert, final re-classification;
  Confirm proves the stored fact or the held proof), the sqlc store
  (`db/queries/trip/events.sql`), the held proof and own-receipt read, and
  `TimelineTx`.
- Migration 000225: forced-RLS, insert-only `org_trip_events` and
  `org_trip_held_events` with same-organization keys to the trip, operation,
  author membership and employee record and enrollment only (no asset, unit
  or cost-centre key: no usage source changes); a BEFORE trigger under the
  root and trip row locks re-proves the operation, the trip author, the zone,
  the window and the identity across both tables; a deferred constraint
  trigger publishes an event only with its `ACCEPTED` revision-1 receipt and
  effect; the held trigger re-proves `TRIP_ALREADY_CLOSED`; the review-source
  hook is redefined from the 000220 body with a `trip.event.record` branch.
- Runtime: `enterprisesuite` registers the owner (`NO_ASYNC_FOLLOWUP` for
  `ACCEPTED`/`REJECTED`, `ATOMIC_REVIEW` for `ACCEPTED_FOR_REVIEW`); OpenAPI
  declares the owner and `EnterpriseTripEventPayload` (apicontract owner count
  4 → 5); the backend README lists the 225 grants. No route, authorization
  matrix, inventory or check-manifest change.

## RED

Shape first (exported names, stubs returning zero values or `ErrUnavailable`,
Kotlin `TODO()`, the frozen oracle and tests, the migration as written).

- Go domain (`trip003-go-domain-red.jsonl`, race): 1/16 top-level failed
  (the new vector test), 101/103 logbook vectors failed. The two passing rows
  (`record-identity-non-canonical`, `record-identity-nil`) are refused by the
  existing scoped-reference constructor before the stubbed function; the same
  two pass in Kotlin. The vocabulary coverage checks were changed from
  `Fatal` to `Error` before the recorded run so every row executes (no
  expectation changed).
- Kotlin (`trip003-kotlin-red.log`, XML in `trip003-kotlin-red-xml`): 389
  trip tests, 101 failed (the same 101 rows).
- Application (`trip003-app-red.jsonl`): 3/8 top-level failed (the three new
  tests). A first run failed one test for the wrong reason (a fixture with an
  invalid zone that the protocol seal itself refuses); the case was removed
  and the RED rerun against the stub; no expectation changed.
- Contract (`trip003-contract-red.jsonl`, previous spec): 2 failed (the new
  owner test and the owner count), 1 passed (TRIP-002 owners).
- PostGIS (`trip003-db-red.jsonl`, stubbed owner and store, real migration):
  6/6 top-level and 4/4 race subtests failed (`RETRY`/`UNAVAILABLE` where a
  receipt was expected). The upgrade test passed its migration half (the
  migration is shape) and failed at the first event on the upgraded schema.

## GREEN and REFACTOR

- Go domain 16/16 top-level, 376 subtests (race), statements 99.7% (the
  uncovered branch is the defensive "trip neither IN_PROGRESS nor COMPLETED",
  unreachable for a constructed trip); application 8/8, 27 subtests, 88.0%;
  adapters unit 5/5 (`trip003-go-green.jsonl`).
- Kotlin enterprise suite (`trip003-kotlin-green.log`, XML in
  `trip003-kotlin-green-xml`): 2783 tests, 0 failures, 0 skipped; the logbook
  vectors 103/103.
- PostGIS logbook tests (`trip003-db-green.jsonl`): one fixture failure — the
  planted foreign trip sent `SET LOCAL` and the insert as one prepared
  statement; split in two, then 6/6 top-level and 16/16 subtests. Every
  `TestTrip*`, `TestFleetImplement*` and `TestFleetAttachment*` PostGIS test
  then passed together, 26/26 top-level and 39/39 subtests
  (`trip003-db-focused.jsonl`).
- Contract focused GREEN after the OpenAPI owner and payload: 5/5
  (`trip003-contract-green.jsonl`).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| Authorized events keep stable identity, actor, original time and zone, receipt (AC) | `TestTripEventIntakeRecordsTheTimelineAndReplays` (SQL proof of every stored column: identity, trip, type, text, author membership and employee, enrollment, digest, sub-microsecond remainder, entry zone, offset, receipt ≤ ACK; an optional text stays NULL) |
| The timeline distinguishes fact type and receipt (AC) | same test: `TimelineTx` returns START, ARRIVAL, NOTE, DEPARTURE, a late STOP, an ARRIVAL at the finish instant and FINISH in order of occurrence; the late STOP was received after the FINISH although it happened before it; texts kept; logbook-v1 ORDER (10 vectors, both languages) |
| Duplicate delivery | resends return the stored ACK byte for byte (accepted and held); `OPERATION_ID_REUSED` for the same id with another digest; `TestTripEventIntakeRollsBackAndConcurrentDuplicatesLeaveOneEvent` (12-way identical deliveries, one fact) |
| Another organization's target | `…/foreign-trip-not-found-pending` (a real row of another organization planted in the same database: `DEPENDENCY_PENDING`, nothing stored, no event on it); `…/unknown-trip-pending`, `…/held-start-trip-pending`; forced RLS hides events without the organization setting |
| An unsupported event changes no trip state | `…/reserved-type-unavailable` (nine reserved types `RETRY`/`UNAVAILABLE`, nothing stored, trip still IN_PROGRESS); `…/unknown-type-malformed`; logbook-v1 TYPE/RECORD; no event changes state, readings or active use (first test) |
| Event before the start / after the finish (EDR-207) | `…/before-start-malformed`; `…/after-finish-held` (held fact, review item, v1 ACK proof, replay, not in the timeline, identity reserved, trip unchanged); late in-window events accepted on a COMPLETED trip (first test) |
| Trip closed by someone else (offline matrix) | held `TRIP_ALREADY_CLOSED` above and in `TestTripEventRacesTheFinishInBothOrders/after-finish-first` |
| Concurrency with the finish, both orders, barriers | `TestTripEventRacesTheFinishInBothOrders` (inside window: event accepted and trip completed in both orders; after the finish: accepted when the event wins, held when the finish wins; lock wait observed each time) |
| Authority | `…/another-member-forbidden`, `…/never-assigned-forbidden` (whole-response `FORBIDDEN`, nothing stored) |
| Identity reuse, malformed payloads | `…/identity-reused-malformed`, `…/note-without-text-malformed`, `…/payload-shape-malformed` (extra key, missing key, non-UUID trip, trip entity type) |
| Atomicity | injected store failure → `RETRY`, nothing stored, the same operation then commits |
| Forced RLS, insert-only, deferred guards | `TestTripEventSchemaIsForcedRLSInsertOnlyAndGuarded` (update/delete refused on both tables; the source trigger refuses a row before the start (`invalid trip event`) and the deferred trigger a row whose operation has no accepted receipt (`trip event publication unavailable`); timeline index) |
| Populated upgrade 000224 → 000225 | `TestTripEventPopulatedUpgradeFailureRecoveryAndPreviousImage` (a completed and an in-progress trip through the real intake; injected failure leaves schema and data unchanged; ordinary forced-RLS migrator; recovery once; no-op replay; previous image compatible; asset usage answer unchanged; review hook keeps all branches; the pre-existing trips take an accepted, a late and a held event) |
| Codec and contract | `trip/application` `TestParseEvent*`, `TestEventDomainAppliesTheLogbookRulesAtReceipt`; adapters `TestEventOwnerPortsRefusalsAndUnboundStore`; apicontract `TestTripEventOwnerIsAnAtomicReviewOwnerWithAnExactPayload` |

## Validation

Go from `backend`, toolchain go1.27.4, `GOCACHE=/data/dev/cache/f4-go-build`,
real PostGIS through `orgtest`, the coordinator MinIO for storage tests:

```sh
gofmt -l ./internal ./cmd ./db                                         # empty
go build ./... && go vet ./... && go vet -tags=integration ./...       # clean
sqlc vet && sqlc diff                                                  # clean (trip stanza + 000225)
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/trip/... ./db/queries/trip ./internal/platform/{enterprisesuite,apicontract} ./internal/modules/organization/adapters   # clean
go test -json -race -tags=integration -p 2 -count=1 ./internal/modules/{organization/adapters,trip/...,fleet/...,orgsync/...} ./internal/platform/{enterprisesuite,apicontract,migrate,orgtest,orgapi} ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
python3 docs/enterprise/tools/check_plan.py                            # problems 0
flock … ./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*'
```

On this commit (pre-merge tree): gofmt, build, vet (both tags), sqlc
vet/diff and staticcheck (plain and integration) are clean
(`trip003-go-static.log`); the focused race tests above pass; Kotlin
enterprise suite 2785/0/0 with the logbook redaction unit tests
(`trip003-kotlin-enterprise-final.log`); OpenAPI vacuum 0 errors (warnings
2900 → 2902, informs 649 unchanged, `trip003-vacuum.log`). A first union run
on this tree was stopped at 540 top-level passes and 0 failures
(`trip003-db-union-premerge-aborted.jsonl`) when the coordinator integrated
EMP-CHECK-001: the union and the untagged suite run on the merged tree
(section "Merged-tree validation").

`git diff --check` is clean; the staged diff was reviewed for secrets and
personal data (synthetic identifiers and Portuguese sample texts only).

## Retained

- EMP-TRIP-006: panel timeline read (route, authorization, five states) over
  `TimelineTx`; EMP-TRIP-005: the Android producer and offline timeline over
  the Kotlin mirror; EMP-TRIP-007/015: resolution of held events and late
  events after approval/amendment (VAL-26), and an event declared after a
  finish that is received before it (accepted while the trip is in progress;
  device dependencies keep the order, OFFL-013 proves convergence).
- Reserved types stay `RETRY`/`UNAVAILABLE` until their owners define the
  payload and effects: EMP-TRIP-010 (planned stops, legs), EMP-TRIP-011
  (pause, resume, cancel), EMP-TRIP-012 (handover), EMP-CHECK-003 (incident,
  damage), EMP-FUEL-011/EMP-EXP-004/EMP-TRIP-020 (fueling, toll, parking).
- Corrections of an event (a new version keeping the old one, 12-ux-app §4.8)
  and photos on events.
- An event whose rights changed after its declaration stores nothing and
  stays `RETRY`/`UNAVAILABLE` (no held owner for `PERMISSION_CHANGED`, as for
  a finish); late intake after access ended stays `RETRY`/`UNAVAILABLE` (no
  `LateHolder`).
- Production grants (backend README); no route, panel, Android, job or
  deployment change; no C1, M4 or F4 acceptance is claimed.
