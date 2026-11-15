# EMP-TRIP-005 — App: start a trip, log events and finish, offline first

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2/C1 Android task (module TRIP, P0, type APP, size L) on
`codex/enterprise-f4-trip005`, based on the F4 integration head `ce984866`
(TRIP-001..004, TRIP-017 and FLEET-013 integrated; migrations through
000226). Synthetic data only; no migration, no new route, no job, no release.
One minimal backend change (EDR-244): the existing context read returns the
member's own employee record. The only Android device used is the owned
emulator `emulator-5588` (AVD `enterprise-f2-api36`, API 36); the physical
phone was never addressed.

Sources read: the EMP-TRIP-005 block and the TRIP-004/006/007/011/015/021
blocks ([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md)
§§2–11 (BR-TRIP-001..004, UC-TRIP-01/02, SCR-TRIP-01);
[12-ux-app](../12-ux-app.md) §§3, 4.3–4.5, 4.8, 4.12, 4.15, 5, 6;
[10-offline-sync](../10-offline-sync.md) BR-OFFL-002/003, UC-OFFL-01/05/08,
the review-code table and the offline matrix rows "Start trip", "Logbook
events", "Finish trip"; [08-state-model](../08-state-model.md) §1–3
(independent operational, sync and review states); the C1 scenario of
[14-roadmap](../14-roadmap.md) §4; the TRIP-001, TRIP-017, TRIP-002,
TRIP-003, TRIP-004, FLEET-013, METER-001/003, OFFL-003, OFFL-004 and
OFFL-008 evidence and code (Go codecs `ParseStart`/`ParseFinish`/
`ParseEvent`/`ParseSubmit`, the intake's SELF-driver classification, the
Kotlin trip domain, the protected queue and its producer projection port, the
sync projection, the FLEET-013 picker and reference cache); EDR-054 (current
membership employee IDs are immutable), EDR-076, EDR-202, EDR-207, EDR-220,
EDR-223..227.

IDs used: B-BR-EMP-375..380, BUC-EMP-189..191, EDR-242..245 (all of them).
No migration number was needed.

## Observed before any change

- The production queue registry was empty: no business producer existed.
  The OFFL-003 producer port (`save`/`acknowledge` projecting
  `EnterpriseLocalRecord`s inside the same protected write) was the designed
  owner of a business record's local index.
- `trip.start` requires `driver_employee_id`, and the intake classifies a
  SELF driver only when it equals the author membership's employee record.
  No device-facing response carried that identifier (context list/read,
  device contact, enrollment, reference feed). The context read already
  loads and checks the membership's employee (`m.EmployeeID`).
- `trip.submit` stays `RETRY` until its finish is stored; a held start keeps
  its events, finish and submission `DEPENDENCY_PENDING` until EMP-TRIP-007.
- The author's own trip review-result read does not exist (TRIP-004
  retained it).
- Context selection is an online read; after a process death the user
  re-selects the organization (existing EMP-IDN/PLAT behaviour), while the
  queued work keeps being delivered in the background by the OFFL-003
  Worker, independent of the UI selection.

## Rules frozen before behaviour

B-BR-EMP-375..380 and BUC-EMP-189..191 are in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-TRIP-005 — App: start a trip, log events and finish, offline
first"; EDR-242..245 in [16-decisions](../16-decisions.md). Both were
appended before any code. The recommended routine decisions, taken
unattended:

- **EDR-242 — local trip storage.** One sealed `RECORD` entry per partition
  (fixed identifier) holding a versioned canonical `trip-book` v1 index, not
  Room tables (EDR-076, PRIV-003, FLEET-013 precedent). It is written only by
  the four registered `trip.*` producers inside the queue's protected write,
  so an operation and its index commit together or not at all; the start
  producer projects a final `REJECTED` start receipt as a refused flag in the
  receipt's own write. Trip facts come back from the immutable operations
  and each record's sync state from its own retained queue entry. The local
  rule for the negative "second active trip on the device for the same
  asset": refused on the device (one unfinished, not refused local trip per
  asset per partition); the server still holds `ASSET_BUSY` across devices.
  Rollback flag `enterpriseTripUiEnabled` (default on).
- **EDR-243 — device run clock and declaration.** `AndroidEnterpriseDeviceClock`
  (`boot-<BOOT_COUNT>`, `SystemClock.elapsedRealtime()`), the lease clock's
  own source, on start and finish; absent when unreadable. Wall instant,
  device zone when the orgtime profile accepts it (else `UTC`) and the real
  offset. Events/finish declared before the start are refused locally; the
  final reading is checked with the METER-003 validator (warning only).
- **EDR-244 — the driver's employee identifier.** `/v1/org-access/contexts/read`
  adds `employee_id` (the requesting membership's own record, already loaded
  and checked by the read); no SQL, migration, route or permission change;
  the list is unchanged. The Android gateway parses it strictly and keeps it
  only in memory with the selected context. The alternatives (reference feed,
  device contact, company-cookie own-assignments route, a new signed route)
  were rejected for their wider wire impact.
- **EDR-245 — presentation, vectors and native acceptance.** Per-record sync
  states from receipts; the review line shows only "Pronto para enviar"
  before and "Aguardando aprovação" after the submission's `ACCEPTED`
  receipt; the author's own review-result read is retained, not invented; a
  held start shows "Precisa de revisão" with its reason and that the trip's
  other records wait. A shared operation oracle pins the producer's bytes
  against the real Go intake parsers.

## Implementation

- **Domain** (`domain/.../enterprise/trip/`, pure):
  `TripOperations.kt` — builders for the four operations with the intake's
  own validation order (ids, METER-001 values and reasons, run clock,
  logbook type unknown `MALFORMED` / reserved `UNSUPPORTED` / text), canonical
  sealing through `EnterpriseOperation.seal`, and a strict decoder `read`
  mirroring `ParseStart`/`ParseEvent`/`ParseFinish`/`ParseSubmit`;
  `TripDeclarations.at`. `TripBook.kt` — the `trip-book` v1 index (strict
  canonical codec, bounds 1000 trips / 500 events / 10 000 operations / 2 MiB,
  binding to membership and enrollment) and its local transitions with the
  closed `TripLocalFailure` vocabulary. `TripPresentation.kt` — trip views
  (records in occurrence order, sync per record, review line, waiting
  notice), refusing a missing or foreign entry. `EnterpriseContextSummary`
  gains an optional in-memory `employeeId`.
- **Application** (`application/.../enterprise/trip/Trips.kt`): the store
  port, `EnterpriseTripStartHints` (picker label for the start producer,
  memory only), `EnterpriseTripProducer` ×4, `EnterpriseTripService`
  (capture conversion with `MeterValues`, local pre-check with the exact
  refusal, build, durable `EnterpriseQueueService.save`; finish warnings) and
  `TripFlow` (one visible owner bound to account, epoch and background lease
  like `MyAssetsFlow`; a failed write reports `saveFailed` and shows no new
  record; "Enviar agora" is one dispatch of the selected partition).
- **Data**: `ProtectedTripStore` (book and the entries it names in one
  `queueAccess` snapshot; an unreadable or foreign book is unavailable and
  never replaced); `EnterpriseContextHttpGateway` parses `employee_id` from
  the read only; DI registers the four producers in the production registry
  and provides the store, service and flow.
- **App**: `TripViewModel` (re-reads the book after each finished delivery
  through `EnterpriseDeliveryActivity.changes`), `TripPanel`/`TripsHomeCard`
  (`app/.../enterprise/trip/TripScreen.kt`): list, start form with the
  FLEET-013 picker (implements filtered out, EDR-219), meter choice or stated
  absence, timeline with one sync chip and reason per record, event grid
  (stop, arrival, departure, note), finish form with the METER-003 warning,
  "Marcar pronta para enviar", "Enviar agora" only with a connection, local
  refusal and "Nada foi salvo" feedback; the logbook notice (LV-05). Wired
  into `EnterpriseContextScreen` with the same partition/member/epoch guards
  and withdrawals as "Meus ativos"; build flag `ENTERPRISE_TRIP_UI_ENABLED`.
  76 strings × 8 locales (`enterprise_trips.xml`, English default and pt-BR
  plus six translations, generated from one table by
  `f4-logs/trip005-scripts/gen_trip_strings.py`); sync labels and reasons
  reuse the OFFL-004/008 strings. Existing container tokens only, no new
  colour.
- **Backend (EDR-244)**: `AccountContextView.EmployeeID`, set by the read
  from the already-checked membership; the read response adds `employee_id`;
  OpenAPI `EnterpriseAccountContextView` gains the required property and its
  example. business-web ignores the additive field.
- **Shared vectors**: `contracts/testdata/enterprise/trip/trip-operations-v1.psv`
  (54 rows: START 8 OK / 17 MALFORMED, EVENT 7 OK / 7 MALFORMED / 3
  UNSUPPORTED, FINISH 4 OK / 4 MALFORMED, SUBMIT 2 OK / 1 MALFORMED / 1
  PROTOCOL; SHA-256 `6c12cb79…3551`), documented in the trip testdata README,
  generated by an independent Python oracle
  (`f4-logs/trip005-scripts/gen_trip_operation_vectors.py`). Go consumer
  `trip/application/operations_vectors_test.go` (real `protocol.ParseOperation`
  + the four intake parsers); Kotlin consumer
  `EnterpriseTripOperationVectorsTest` (builders byte-for-byte, decoder,
  refusal classes).
- **Native fixture** (test-only, tags `integration && nativefixture`):
  `organization/adapters/orgsync_trip_native_fixture_test.go`, the actual
  default enterprise runtime over an orgtest child database of the owned
  PostGIS (`127.0.0.1:32775`), signed HTTPS with a disposable CA, two
  organizations, a second assigned member of A.

- **Defect fixed on the way (EDR-244):** the first native run stayed
  "Salvo no aparelho" after reconnection. A temporary, uncommitted stderr
  trace in the start handler (reverted before any gate) showed the real
  runtime answering the start `RETRY` `UNAVAILABLE` from `record`: with the
  runtime's METER-003 plausibility on, the departure reading (184 320 km six
  hours after the synthetic opening reading of 0 km) is stored `SUSPECT`, and
  the trip intake's `readingRecorded` accepted only `VALID` — unlike the meter
  and fueling intakes. A trip with any implausible or decreasing reading could
  never be delivered. `trip/adapters/intake.go` now accepts `VALID` or
  `SUSPECT`, exactly like them (B-BR-EMP-280..285 keep a suspect reading
  aside, never refused); B-BR-EMP-379 says so.

## Validation — RED → GREEN and gates

Logs in `/data/dev/worktrees/f4-logs/trip005-*`. Gradle always ran with JDK
17, `--offline --no-daemon --no-parallel --max-workers=2` and the comaps path
under `flock /data/dev/worktrees/.f4-gradle.lock`; Go with the go1.27.4
toolchain, `GOCACHE=/data/dev/cache/f4-go-build`, real PostGIS through
`orgtest` child databases of `127.0.0.1:32775`.

- **Domain RED** (`trip005-kotlin-domain-red.log`, XML kept): 63 tests, 62
  failing on `NotImplementedError` — vectors 54/55 (only the corpus/coverage
  check, which reads no builder, passed) and trip book/presentation 8/8.
- **Domain GREEN** (`trip005-kotlin-domain-green-1.log`): 63/63 on the first
  implementation run (every expected byte came from the independent oracle).
- **Application RED** (`trip005-kotlin-application-red.log`, XML kept):
  12/12 failing on stubs → **GREEN** 12/12 on the first implementation run.
- **Data RED** (`trip005-kotlin-data-red.log`, XML kept): store 4/5, process
  death 1/1, gateway 1/8 (the new test) failing for the right reasons; the
  fifth store test (no plaintext trip fact in the sealed files) passed against
  the stub because sealing belongs to the existing vault — not claimed as RED.
  **GREEN**: 5/5, 1/1, 8/8 after one fixture correction (two loop iterations
  reused one vault directory with fresh test keys and read as unrecoverable;
  each iteration now has its own directory — no expectation changed).
- **App ViewModel RED** (`trip005-kotlin-app-red.log`, stubbed ViewModel,
  XML kept): 4/4 → **GREEN** 4/4 with the real ViewModel.
- **Enterprise host suites** after GREEN (`trip005-kotlin-enterprise-suites-1.log`,
  XML kept): domain enterprise 3118, application enterprise 120, data
  enterprise 183, app enterprise 23 tests — 0 failures, errors or skips.
- **Go shared vectors** (`trip005-go-vectors-2.jsonl`): 1 top-level / 54
  subtests pass. The consumer pins existing parsers, so it has no behavioural
  RED; its first run (`-1`) correctly failed one row whose zero revision the
  envelope profile itself refuses, which became the `PROTOCOL` class (oracle
  regenerated, no Go behaviour changed).
- **Go context read RED → GREEN**: `TestAccountContextsThroughActualCompanyHTTP`
  failed on the missing `employee_id` (`trip005-go-context-red.jsonl`) →
  GREEN with every `TestAccountContext*` and the OpenAPI-validated
  `TestReviewCorrectionsActualCompanyHTTPMatchesContract`
  (`trip005-go-context-green.jsonl`, 9 top-level); `apicontract` and `cmd/api`
  pass.
- **Go suspect reading RED → GREEN**:
  `TestTripIntakeAcceptsSuspectTripReadingsWithThePlausibilityPolicyOn`
  failed `orgsync: unavailable` (`trip005-go-suspect-red.jsonl`) → GREEN with
  every trip PostGIS test, 28 top-level / 59 named (`trip005-go-suspect-green.jsonl`).
- Go unit race on trip and organization application/http
  (`trip005-go-unit-race.jsonl`): 6 packages, 583 named, 0 failures.
- Toolchain gofmt clean; `go build ./... && go vet ./... && go vet
  -tags=integration ./...` and `go vet -tags='integration nativefixture'` on
  organization/adapters pass; staticcheck (go1.27.1) plain, `-tags=integration`
  on organization and trip and `-tags='integration nativefixture'` on
  organization/adapters find nothing (`trip005-go-static-final.log`).
- No SQL, migration, sqlc input, route or permission changed, so `sqlc`, the
  route inventory, the authorization matrix and the AUDITOR walk were not
  touched.

- **Race/integration union** with PostGIS and a throwaway MinIO project
  (`f4-trip005-s3` from `infra/compose.validation.yml`, removed with `down
  -v` afterwards) over organization, trip, fleet, orgsync, apicontract,
  enterprisesuite and cmd/api (`trip005-db-union.jsonl`, `-race -p 2`, 30.1
  min, organization/adapters 1804 s): 20 packages pass, 959 top-level / 8082
  named tests, 0 failures, 0 skips.
- Untagged `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  (`trip005-untagged-all.log`): 222 packages ok, exit 0, 2m34s.
- Android: normal `:app:testDebugUnitTest` (enterprise) `:app:assembleDebug
  :app:lintDebug :data:lintDebug` (`trip005-lint-normal-build-final.log`):
  app enterprise 23 tests pass; lint 0 errors, 352 app and 17 data existing
  warnings, none in touched sources (an earlier run's two new warnings —
  three unused strings and a boxed `Int` state — were fixed); enterprise
  validation builds `:app:assembleDebug :app:assembleDebugAndroidTest
  :data:assembleDebugAndroidTest -PenterpriseDeviceValidation=true` pass.
- `python3 docs/enterprise/tools/check_plan.py`: 0 problems. `git diff
  --check` clean. Staged diff reviewed for secrets/PII: synthetic identifiers
  and names only; the native fixture's proof, CA and session stayed in a
  mode-0700 `/data/dev/cache` directory and the test app's private storage,
  both removed afterwards; the fixture logs only acknowledgement bodies
  (operation identifiers, digests, results, codes), never a request.

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the screens and services.

- **Real signed backend** (`trip005-native-source.log`,
  `trip005-native-device.log`, driver `trip005-scripts/native-driver.sh`):
  `TestTripNativeSourceFixture` PASS (105.5 s) with
  `EnterpriseTripNativeDeviceTest` in five processes, the driver force-stopping
  only the target between every stage. Stage 1 (route present): custody of
  both organizations through the real contact route and the signed reference
  pull of A. Route removed. Stage 2 (new process, no route): the vehicle chosen
  from the protected cache with no network, start with the 184 320 km
  departure and a STOP saved through the product's queue and producers, both
  "Salvo no aparelho"; a dispatch without route received nothing and changed
  nothing. Stage 3 (new process, no route): the same timeline after process
  death, NOTE, finish at 184 610 km and "ready to send" saved, review line
  "Pronto para enviar"; organization B shows no trip; every queued operation
  sealed to A's organization and membership; no trip identifier, note text,
  value, operation type or book format readable outside encryption. Route
  restored. Stage 4 (new process): each record moved from "Salvo no aparelho"
  to "Aceito pela empresa" only with its own receipt, review line "Aguardando
  aprovação"; a second dispatch sent nothing. A second trip on the same
  vehicle (allowed locally: the first is finished) and its STOP saved. The
  fixture then let the second assigned member's enrolled device start a trip
  on the vehicle through the real intake. Stage 5 (new process): the device's
  second start came back `ACCEPTED_FOR_REVIEW` `ASSET_BUSY`, shown "Precisa de
  revisão" with that code, the trip says its STOP waits, the first trip stays
  "Aguardando aprovação". Server: intake deliveries A = 4 × 200, B none;
  org A: 1 trip of this member (COMPLETED), 1 finish, 2 events, 1 SUBMIT, 1
  held start (review item `ASSET_BUSY`), 6 operations and 6 receipts from this
  member — every record stored exactly once. Cleanup removed both partitions,
  the identity/session keys, the private fixture and the route.
  Earlier attempts: run 1 and a diagnostic run failed at stage 4 on the
  suspect-reading defect above (logs `-run1`, `-debug`); after the fix the
  whole run passed first time. The second member's device is a real enrolled
  device source of the fixture through the same intake store, not a second
  emulator: a two-physical-device run is OWED.
- **Presentation** `EnterpriseTripPresentationDeviceTest` 6/6
  (`trip005-native-presentation-3.log`, pt-BR, real trip views from real
  operations and receipts): saved records never shown accepted and "Enviar
  agora" disabled offline; each record changes only with its own receipt,
  "Aguardando aprovação" with no approval claimed; held start with reason and
  waiting notice (state description carries status and reason); start through
  the FLEET-013 picker with a decimal-comma entry, local refusal keeps the
  form, a durable save returns with "Salvo no aparelho."; event grid, finish
  with the METER-003 warning still saving, "ready" only after finish; a failed
  write says "Nada foi salvo". First runs: one process crash from a stale
  WorkManager job of the validation app left by earlier sessions (cancelled
  with `cmd jobscheduler cancel` on the owned emulator), and one failure where
  a pointer click on "Enviar agora" missed because the whole open trip was a
  single lazy item taller than the screen; the open trip is now one lazy item
  per record (a product change for long timelines), and the test is unchanged
  apart from asserting the button is enabled.
- **Regressions** (`trip005-native-app-regression.log`) on the same APK pair:
  `EnterpriseMyAssetsPresentationDeviceTest` 6/6,
  `EnterpriseSyncPresentationDeviceTest` 6/6,
  `EnterpriseReviewPresentationDeviceTest` 4/4, the five
  `EnterpriseContextDeviceTest` methods (real Hilt UI graph incl. the new
  `TripViewModel` binding) 1/1 each, and `EnterpriseSyncContextDeviceTest`
  (now passing a `TripViewModel` over the same protected store; it builds
  `EnterpriseContextScreen` without Hilt) — its first class run failed
  `myAssetsShow…` with a STALE assets owner after the first method (the
  intermittent FLEET-013 retained); the method passed alone 2/2 and the class
  3/3 afterwards.
- APK SHA-256 (`trip005-validation-apks.sha256`): validation app
  `1afea1ec…542d`, validation test `0ab1ea85…3f7f`, data test `28e57a24…7b80`.

## Named negatives → tests

| Negative / acceptance | Test(s) |
|---|---|
| After restart, the original context and the timeline remain | `EnterpriseTripFlowTest` restart test; `ProtectedTripProcessTest` (child JVM halted before the durable replace and after the trusted head: operation and index together or neither); native stages 2→3→4 across force-stops |
| Acceptance follows server receipts, never early | `EnterpriseTripLocalTest` receipt test; flow delivery test; presentation receipt tests; native stages 2–4 |
| Completion shown as accepted early | presentation offline test (no "Aceito"); review line only from the submission's ACCEPTED receipt (flow held test keeps "Pronto para enviar") |
| Context switch retags queued work | flow BR-OFFL-003 test (B sees nothing, bytes unchanged, A delivered to A); producers bound by the queue's attempt check; native stage 3 (B empty, all A operations bound to A) |
| Local save reported without durable persistence | flow failed-write test (no record, `saveFailed`); store producer-refusal test; presentation failed-write test |
| Second active trip on the device for the same asset | `TripBook` rule test; flow refusal test (nothing queued); store test (producer refuses inside the write); presentation refusal text |
| Server holds `ASSET_BUSY`, `ASSET_UNAVAILABLE`, `TRIP_ALREADY_CLOSED`, `PERMISSION_CHANGED` as "Precisa de revisão" with reason | `EnterpriseTripLocalTest` held test (all four codes); flow held test; presentation held test; native stage 5 (real `ASSET_BUSY`) |
| A finally refused start frees the asset | flow refused-start test; store projection test |
| Clock moved back / event before start | `TripBook` test; flow `BEFORE_START` test |
| Driver identifier missing or malformed | flow test; gateway test (strict, list never supplies it); Go context read test |
| Producer bytes are exactly what the intake parses | shared vectors (Kotlin builders/decoder, Go `ParseStart`/`ParseEvent`/`ParseFinish`/`ParseSubmit`) |
| Foreign/unreadable book or other enrollment | store foreign-book test; codec strictness test |
| Another account on the same device | store test (denied; empty own partition) |
| Suspect trip reading never blocks the trip | `TestTripIntakeAcceptsSuspectTripReadingsWithThePlausibilityPolicyOn`; native stages 4 |

## Retained (with reasons; not part of this task's acceptance)

- The author's own trip review-result read (approved / returned / rejected):
  no server read exists (TRIP-004 retained it); the screen says the decision
  is not received on this device. Owner: the EMP-TRIP-015 correction loop or
  a dedicated read owner.
- Resolution of a held start and its waiting children (EMP-TRIP-007); until
  then they stay saved and retried with the OFFL-003 backoff (cap 6 h).
- Re-selecting an organization offline after a process death: context
  selection is an existing online read (EMP-IDN/PLAT); the trip book and the
  background delivery do not depend on it, and the native proof of restart is
  at the data level through the product's services, not through the context
  screen.
- The current-activity card and trip return bar (12-ux-app §4.4, §5.2), the
  driving layout (§5.5), event corrections and photos, reserved event types,
  pause/resume/cancel, missions, tracking, checklists and the closing
  conference (EMP-TRIP-010/011/012/020/021, EMP-CHECK-002/003, tracking).
- A two-physical-device concurrent start: OWED (proved with a second enrolled
  member's device source through the same real intake).
- Native other-account and 24-hour boundaries for the trip book (host-proven
  with the real protected store).
- A departure-reading plausibility warning at the start (the feed carries no
  previous reading, EDR-216); only the finish is checked against the
  departure.
- Retention/pruning of the trip book and of final receipts (no owning policy
  yet, like OFFL-003).
- The `EnterpriseSyncContextDeviceTest` intermittent (FLEET-013), seen once.
- Rendered light/dark/200 % font review (`VISUAL_VALIDATION_PENDING`) and a
  TalkBack pass on SCR-TRIP-01.
- Merged-tree verification, production grants (none new beyond those TRIP-002/
  003/004 retained) and any staging/release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the FLEET-013 sentence:
   "Trips are recorded offline as signed `trip.*` operations whose sealed
   per-partition index commits with each operation (EMP-TRIP-005,
   EDR-242..245)."
2. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`, after the
   EMP-FLEET-013 paragraph: "EMP-TRIP-005 registers the first business
   producers (`trip.start`, `trip.event.record`, `trip.finish`, `trip.submit`);
   their projection is one sealed `RECORD` per partition (`trip-book` v1,
   bound to membership and enrollment) written in the same protected write as
   each operation; no plaintext table exists."
