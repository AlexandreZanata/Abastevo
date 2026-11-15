# EMP-FLEET-013 — App: my assets and asset selection

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2/C1 Android task (module FLEET, P0, size S) on
`codex/enterprise-f4-fleet013`, based on the F4 integration head `76ff7b27`
(migrations through `000224`). Synthetic data only; no backend production
change, no migration, no route, no OpenAPI change, no job, no release. The only
Android device used is the owned emulator `emulator-5588` (AVD
`enterprise-f2-api36`, API 36); the physical phone was never addressed.

Sources read: the EMP-FLEET-013 block of the backlog; [fleet](../modules/fleet.md)
§3, §4 (BR-FLEET-024/027), §5 (UC-FLEET-08), §6.2, §8 and §11
(SCR-FLEET-08/09/10); [offline sync](../10-offline-sync.md) §4 (BR-OFFL-016)
and §10; BR-TEN-004; [12-ux-app](../12-ux-app.md) §3, §4.5, §5;
`docs/mobile/UI_CONTAINER_GUIDELINES.md`; the OFFL-006 evidence, wire
(`contracts/openapi/v1.yaml` `EnterpriseReferenceFeed*`) and server code
(`orgsync/application/reference.go`, `orgsync/adapters/reference.go`,
`orgsync/adapters/http/reference.go`, `fleet/adapters/reference.go`); the
Android precedents PLAT-011, IDN-018, PRIV-003 (V1 and completion), OFFL-003,
OFFL-004 and OFFL-008 (signed review-results client and native closure);
EDR-076 (no second Room store).

IDs used: B-BR-EMP-348..352, BUC-EMP-175..177, EDR-224..227 (all of them).
No migration number was needed.

## Observed before any change

- No Android consumer of the reference feed existed; OFFL-006 retained the
  Kotlin client and the shared wire vectors to this task.
- The enterprise Room database (`EnterpriseDatabase`) is a synthetic
  foundation absent from production DI; PRIV-003 forbids any plaintext
  Room/WAL/journal fallback and EDR-076 already rejected a second Room store.
  The accepted store is the sealed protected generation, which already holds
  a `CACHE` entry kind.
- The feed (EDR-216) carries assets, accepted products, in-service meters with
  their open segment and the covering assignment — no readings, no tank
  capacity and no availability beyond the lifecycle state.
- The app had no enterprise rollback flag; `QUALITY_UI_ENABLED` and
  `OCCURRENCE_UI_ENABLED` are the existing build-flag pattern.

## Rules frozen before behaviour

B-BR-EMP-348..352 and BUC-EMP-175..177 are in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-FLEET-013 — App: my assets and asset selection"; EDR-224..227 in
[16-decisions](../16-decisions.md), both appended before any code.

- EDR-224: the "local tables" are one versioned reference cache sealed in the
  exact account/origin/organization protected generation (one `CACHE` entry
  with a fixed identifier), bound to membership and enrollment; the "schema
  migration test" is the versioned cache codec (pinned by the shared vectors)
  plus the pre-existing-generation, unknown-version, foreign-enrollment and
  process-death tests.
- EDR-225: strict whole-response client; bound to the request and held
  revision; RESET/DELTA/tombstone/FORBIDDEN semantics; one compare-and-set
  protected commit; a `NOT_FOUND` with a cursor retried once without one;
  2 MiB response bound; pulls on open-while-connected and explicit refresh.
- EDR-226: honest presentation (server `confirmed_at`; 24 h by elapsed time
  within one boot or wall time after reboot; per-asset assignment end;
  refresh-failure notice; never hide/disable on the device clock; lifecycle
  is the only availability fact); the picker allows not available / not
  confirmed assets after a review warning (AC3, SCR-FLEET-09, BR-FLEET-027),
  superseding "listed disabled" of 12-ux-app §4.5 step 3 for this task;
  build flag `enterpriseFleetUiEnabled` (default on) is the rollback switch.
- EDR-227: shared oracle consumed by Go and Kotlin; APPLY/CACHE semantics are
  client-only; native acceptance against an ordinary-PostGIS signed fixture;
  readings and tank capacity retained with a future entity-type owner.

## Implementation

- **Domain** (`domain/.../enterprise/fleet/ReferenceFeed.kt`, pure): request
  builder/parser (canonical bytes, 4 KiB), strict response parser bound to
  request and held revision (exact keys, closed enums, OpenAPI bounds counted
  in code points, canonical UTC instants, Go zero instant refused, structure
  checks before correlation), `ReferenceCache` v1 (canonical JSON incl. device
  receipt clock; strict decode; `apply` and `heldRevision`), and `MyAssets`
  (confirmation, presentation with availability/assignment-end facts, picker
  choice). `CanonicalJson.parseLarge` adds the 2 MiB reference profile without
  changing the 64 KiB V1 parser.
- **Application** (`application/.../enterprise/fleet/MyAssets.kt`): ports
  `EnterpriseReferenceCacheStore` (read, CAS replace) and
  `EnterpriseReferenceTransport`; `EnterpriseReferenceService` (offline read,
  online refresh with one 404 retry, CAS commit, concurrent winner shown
  whole); `MyAssetsFlow` (one visible owner bound to epoch, background lease
  and generation like `EnterpriseSyncFlow`; offline-first open; failed pulls
  keep the list; `choose` needs no I/O).
- **Data**: `ProtectedReferenceCacheStore` (sealed `CACHE` entry via the
  existing `queueAccess` custody/enrollment/lease checks; unreadable or
  foreign caches read as absent) and `EnterpriseReferenceHttpClient` (same
  signed device plane as the review client: no cookies/redirects/cache,
  `no-store` required, `Retry-After` profile, 404 distinct, response bound
  before reading). Hilt providers in `EnterpriseModule`.
- **App**: `MyAssetsViewModel`; `MyAssetsPanel` (SCR-FLEET-08 five states and
  the SCR-FLEET-10 read-only detail), `EnterpriseAssetPicker` (SCR-FLEET-09,
  caller filter, ≥72 dp rows, review warning dialog), `MyAssetsHomeCard` on the
  enterprise home, wired into `EnterpriseContextScreen` with the same exact
  partition/member/epoch guards and withdrawals as the sync panel; 53 strings
  in all eight locales (`enterprise_assets.xml`); existing container tokens
  (`surfaceContainerLow`, `AnpContainerDefaults`), no new colour; build flag
  `ENTERPRISE_FLEET_UI_ENABLED`.
- **Shared vectors**: `contracts/testdata/enterprise/sync/reference-feed-v1.psv`
  (129 rows: REQUEST 23, RESPONSE 79, APPLY 16, CACHE 11) and `REFERENCE.md`,
  generated by an independent Python oracle
  (`/data/dev/worktrees/f4-logs/fleet013-scripts/gen_reference_vectors.py`,
  Go-style HTML-escaped wire, canonical expected bytes). Go consumer
  `orgsync/adapters/http/reference_vectors_test.go` runs REQUEST rows through
  the real `parseReferenceRequest` (and the 4 KiB body bound), proves every
  OK/BINDING response re-encodes byte-for-byte through `referenceResponseWire`
  and meets the server delta invariants, every INVALID/LIMIT response is not
  server-producible, and that a canonical projection never passes as server
  output. Kotlin consumer `ReferenceFeedVectorsTest` runs every row.
- **Native fixture** (test-only, tags `integration && nativefixture`):
  `organization/adapters/orgsync_reference_native_fixture_test.go`, an actual
  default enterprise runtime over an orgtest child database of the owned
  PostGIS (`127.0.0.1:32775`), signed HTTPS with a disposable CA.

## Validation — RED → GREEN and gates

Logs in `/data/dev/worktrees/f4-logs/fleet013-*`. Gradle always ran with
JDK 17, `--offline --no-daemon --no-parallel --max-workers=2` and the comaps
path under `flock /data/dev/worktrees/.f4-gradle.lock`.

- RED domain (`fleet013-kotlin-domain-red.log`, XML kept): `MyAssetsTest` 7/7
  and all 129 vector cases failed on `NotImplementedError`; only the loader
  grammar test (no behaviour) passed.
- GREEN domain (`fleet013-kotlin-domain-green-1.log`): 137/137 on the first
  implementation run (every expected byte came from the independent oracle).
- RED host (`fleet013-kotlin-host-red.log`, XML kept): application 12/12,
  data 11/11 (HTTP 5, cache store 5, process death 1), app ViewModel 4/4 — all
  failing against stubs.
- GREEN host: application 12/12 and data 11/11 on their first runs; app
  ViewModel 3/4: the `withdrawal and clearing` test counted the documented
  initial epoch collection as an extra `invalidateIfStale`; the fixture now
  clears mocks after construction (like `EnterpriseSyncViewModelTest`); the
  expectation (exactly one forwarded call) is unchanged → 4/4.
- AC5 guard `MyAssetsPersonalIsolationTest` (static: no FLEET-013 source
  references the personal vehicle model, tables or database): 1/1.
- Enterprise host suites after GREEN (`fleet013-kotlin-enterprise-suites.log`):
  domain enterprise 2950, application enterprise 108, data enterprise 176,
  app enterprise 19 tests — 0 failures, errors or skips.
- Normal `:app:assembleDebug :app:lintDebug :data:lintDebug`
  (`fleet013-lint-normal-build.log`): 0 errors; 352 app and 17 data existing
  warnings, none in touched files; no missing translation.
- Enterprise validation build `:app:assembleDebug :app:assembleDebugAndroidTest
  -PenterpriseDeviceValidation=true` and `:data:assembleDebugAndroidTest` pass.
- Go: toolchain gofmt clean (one alignment fix in the new test file); `go
  build ./... && go vet ./... && go vet -tags=integration ./...` and `go vet
  -tags='integration nativefixture'` on organization/adapters pass;
  staticcheck (go1.27.1) plain and `-tags=integration` on orgsync/adapters/http
  and `-tags='integration nativefixture'` on organization/adapters find
  nothing (`fleet013-staticcheck.log`); vectors 1 top-level / 129 subtests
  (`fleet013-go-vectors-1.jsonl`); `-race` on orgsync/adapters/http 7
  top-level / 136 named pass (`fleet013-go-http-race.jsonl`); untagged
  `go test -count=1 -p 2 -timeout=45m ./...` (`fleet013-untagged-all.log`):
  221 packages ok, exit 0, 3m55s. The Go vector consumer pins an existing server, so it has no
  behavioural RED; its discrimination is the canonical-projection assertion.
- No SQL, migration, route, OpenAPI or sqlc input changed, so the
  organization/adapters race union, the route inventory, the authorization
  matrix and `sqlc` were not run (the only file added there is the opt-in
  nativefixture test, vetted and staticchecked with its tags).
- `python3 docs/enterprise/tools/check_plan.py`: 0 problems. `git diff
  --check` clean. Staged diff reviewed for secrets/PII: synthetic identifiers
  and names only; the native fixture's proof, CA and session stayed in a
  mode-0700 `/data/dev/cache` directory and the test app's private storage,
  both removed afterwards.

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the screens/client.

- `EnterpriseMyAssetsPresentationDeviceTest` 6/6 (`fleet013-native-presentation-2.log`):
  loading, error/retry, needs-connection, empty naming who can assign,
  FORBIDDEN without data, offline list with confirmation time, not-confirmed,
  refresh-failed and offline notices, unavailable state text and state
  description, read-only detail (products, odometer, offline limits), and the
  picker returning to a minimal calling screen through the real `MyAssetsFlow`
  with no network (warning cancel → nothing; warning confirm → chosen with
  the warning recorded; zero feed requests); a confirmed asset is returned at
  once and the caller filter hides others. First run 4/6: the two failures
  asserted text on non-merging container cards; the assertions now match the
  same texts on descendants (no expectation changed).
- `EnterpriseSyncContextDeviceTest` 2/2: the existing protected status method
  (now passing the assets ViewModel) and the new
  `myAssetsShowOnlyTheSelectedOrganizationsProtectedCacheOnlineAndOffline`
  through the actual context screen and Keystore-protected cache: AC1 online
  pull, AC2 offline reopen with no feed contact and refresh disabled, AC4
  switch to B withdraws A's list and shows only B's asset, stopped screen keeps
  no assets, AC5 personal `vehicle` rows identical before/after.
  An early run of the new method used the real delivery-proof refresh against
  the synthetic account API, whose in-process family did not match the
  retained session; the product correctly treated the answer as
  `SESSION_REVOKED` and erased the synthetic validation account's vault
  (account denial). The synthetic state was restored once through the
  product's own acknowledged empty recovery (a temporary, uncommitted
  instrumentation that confirmed a fresh partition with
  `acknowledgeErasedWork`, verified a second fresh partition confirms without
  it, and removed both); the fixture now aligns the synthetic family with the
  retained session before any refresh. Afterwards the class passed 9 of 10
  runs: one run inside a regression loop failed with only its summary kept;
  the same order was replayed twice and the class rerun 6 more times, all
  passing (`fleet013-native-context-*.log`). The intermittent is retained.
- Regression on the same APK pair (`fleet013-native-app-regression.log`):
  `EnterpriseSyncPresentationDeviceTest` 6/6,
  `EnterpriseReviewPresentationDeviceTest` 4/4 and the four independent
  `EnterpriseContextDeviceTest` methods (real Hilt UI graph incl. the new
  `MyAssetsViewModel` binding) 1/1 each.
- Real signed backend (`fleet013-native-source.log`, `fleet013-native-device.log`,
  driver `fleet013-scripts/native-driver.sh`): `TestReferenceFeedNativeSourceFixture`
  PASS (250.77 s) with `EnterpriseReferenceFeedDeviceTest` in three processes:
  stage 1 enrolled the device in both organizations through the real contact
  route and pulled A (PRIMARY asset with its meter/segment and a TEMPORARY
  asset ending in 4 min; another driver's asset absent) and B (its own asset
  only), with no cached name, identifier or credential outside encryption;
  the driver force-stopped only the target and removed the `adb reverse`
  route; the fixture then ended the PRIMARY assignment through the real
  FLEET-006 store; stage 2 (new process, no route, after the TEMPORARY end)
  read the byte-identical protected cache, still listed both assets, marked
  the expired one not confirmed with a review warning, returned the picker
  choice with no network and kept the cache unchanged after a failed pull in
  the actual reference client; stage 3 (route restored) received both
  tombstones (A empty) while B kept its asset. Server: A logged 2 upserts and
  2 deletes, B 1 upsert; exactly 2 pulls per organization, all 200 — nothing
  reached the source while offline. The driver's first success check misread
  the instrumentation tail after stage 1 (stage 1 itself printed `OK (1
  test)`); it was corrected and resumed from stage 2 within the same fixture
  session. Cleanup removed both partitions, the identity/session keys, the
  private fixture and the reverse route.
- APK SHA-256: validation app `03270d15eddc41a3e1fda9dc43d39efe495139807312bc9c302e464d1ceed629`,
  validation test `9683d392e3f3131f32d398eac960cb97fb09195e4943c66b7630c912345709f1`,
  data test `69df1555f60c7c327acc8c0d7218be0551fd883e9d84f7f18fa859f5c43d1bd3`.

## Named negatives → tests

| Negative / acceptance | Test(s) |
|---|---|
| AC1 exactly the assets with an active assignment | `EnterpriseMyAssetsFlowTest` AC1; native stage 1; context device method |
| AC2 offline, confirmation time, usable | flow AC2; presentation offline test; context device method; native stage 2 |
| AC3 not available chosen after the warning | `MyAssetsTest` picker; flow AC3; presentation picker tests; native stage 2 |
| AC4 organization switch shows only its assets | flow AC4; `ProtectedReferenceCacheStoreTest` partition test; context device method; native stage 1 |
| AC5 personal vehicle list unchanged | `MyAssetsPersonalIsolationTest`; context device method (vehicle rows before/after) |
| No assignment → empty state naming who can assign | presentation empty test; flow empty test |
| Assignment ended on the server while offline → still listed, not confirmed | flow AC2; `MyAssetsTest` presentation; native stages 2–3 |
| Another account on the same device sees nothing | `ProtectedReferenceCacheStoreTest` (real protected store, test keys); flow account/epoch test |
| Process death while the list refreshes | `ProtectedReferenceProcessTest` (child JVM halted before the durable replace and after the trusted head: exactly old or new cache); flow process-death test; native force-stops between stages |
| Gap, malformed, foreign or oversized responses change nothing | shared vectors (BINDING/INVALID/LIMIT); `EnterpriseReferenceHttpClientTest`; flow failed-refresh test |
| Lost cursor (404) | flow retry test (exactly one retry, none without a cursor) |
| Concurrent refreshes | flow CAS test; store CAS test |
| FORBIDDEN drops everything | vectors; flow FORBIDDEN test; presentation forbidden test |
| Unknown cache version / other enrollment / pre-existing generation | vectors CACHE rows; store tests |

## Retained (with reasons; not part of this task's acceptance)

- Last known readings and tank capacity (SCR-FLEET-08/10, UC-FLEET-08): the
  feed carries none (EDR-216); a readings/tank entity type needs its owner.
  The detail says so on screen.
- Availability beyond the lifecycle state (reservations, blocks, maintenance,
  trips in use): a FLEET-007 availability entity type in the feed.
- SCR-FLEET-10 "Record a reading" (SCR-METER-01 owner), starting an operation
  from "Meus ativos" and the picker's callers (EMP-TRIP-005, EMP-FUEL-011,
  CHECK-002), including §4.5 single-asset preselection and search.
- Background/periodic reference pulls: none added; pulls run on open while
  connected and on explicit refresh.
- Native two-account isolation of this cache specifically and the native
  24-hour boundary were not run (host-proven with the real protected store and
  synthetic clocks; OFFL-003 native key-prefix isolation is the precedent).
- The one unreproduced intermittent of `EnterpriseSyncContextDeviceTest`
  inside a regression loop (summary only).
- Rendered light/dark/200 % font review (`VISUAL_VALIDATION_PENDING`) and a
  TalkBack pass on the new screens.
- Merged-tree verification, production grants and any staging/release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the reference-feed
   sentence: "The app keeps it as one versioned cache sealed in the protected
   partition and shows its server confirmation (EMP-FLEET-013, EDR-224..226)."
2. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`, after the
   EMP-OFFL-003 paragraph: "EMP-FLEET-013 stores the signed reference feed as
   one sealed `CACHE` entry per partition (versioned canonical JSON, bound to
   membership and enrollment, replaced by compare-and-set); no plaintext table
   exists."
