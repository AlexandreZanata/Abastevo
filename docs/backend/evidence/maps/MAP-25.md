# MAP-25 — Private favorites, saved trips and safe file exchange

Status: LOCAL_DONE (see "Not delivered")
Validation: PASS for the delivered scope; the cloud backup/restore cases are NOT delivered
Integration: INTEGRATION_PENDING

Parent: `beed14ab` (MAP-24). Contract: [MAP_25_CONTRACT](../../../product/MAP_25_CONTRACT.md), written first. No payment, MAP-26+ (except where stated), PAY, ESP, VPS/deploy. Synthetic coordinates and ids only; nothing uploaded.

## Delivered

- **Domain** (`domain/map/saved`): exact micro-degree `GeoPoint` (refuses, never clamps), `SavedPlace` (station favorites keep the station UUID, compatible with the cloud-sync `favorites` `{station_id}` shape), `SavedTrip` (ROUTE_PLAN stores only the plan and avoid options; RECORDED/IMPORTED tracks), bounds (5,000 places, 500 trips, 10,000 points, 102 plan points), `SavedOwner` (guest or account + origin).
- **Safe exchange**: in-repo strict XML subset reader (no dependency admitted): DOCTYPE/ENTITY/PI/non-UTF-8/user entities/depth>64/>200k elements/>5 MiB refused; GPX 1.1 and KML 2.2 read and written with original ids, source ids, avoid options and a license/privacy notice; KMZ read in memory only (≤64 entries, ≤20 MiB expanded, no nested archives, no absolute/`..`/backslash/drive names, root `.kml` only). Import is all-or-nothing, idempotent by content hash, never overwrites an existing id.
- **Storage**: Room v28 (migration 27→28, append-only, composite owner/origin keys so two owners can hold the same record id), tombstones keep ids only, import receipts, recording rows. Excluded from cloud backup/device transfer by the existing `database` exclusion rules.
- **Application/UI**: owner resolved per call with an account-switch fence; station favorite toggle in the map sheet; "Save route plan" in the route sheet; Saved screen (list, per-item export, export all, import through the system chooser, deletion with an explicit second confirmation for previously deleted ids, privacy warning); explicit recorder `TripRecordingService` (foreground, location type, notification Stop action, not sticky, quality-gated points ≥10 m apart, refuses to start while guidance runs). Strings in the eight locales.

## Evidence

- Pure/JVM: 26 new domain tests (coordinates hand-calculated, XXE/billion-laughs/PI/encoding/structure/limits, foreign GPX/KML, round trips, KMZ traversal/nesting/bomb/entries), 7 use-case tests (owner fence, malicious file never reaches storage, export only the owner's data, recorder gating), 5 ViewModel tests. Mutation checks: disabling DOCTYPE refusal, traversal check, zip-bomb limit or point limit each fails 2–4 tests; removing the owner filter from two DAO queries fails 3 Room tests.
- Real Room on the owned emulator (API 35, 16 KB): `RoomSavedStoreInstrumentedTest` 9/9 (idempotent favorite, restart, A/B/guest isolation, concurrent saves → one row, 5,000-place quota refusal without eviction and atomic import, 10,000-point trip round trip + cascade delete without touching another owner, import idempotency, tombstone/resurrection with and without confirmation, recording lifecycle and limit, clear owner); `V27ToV28DatabaseMigrationTest` 1/1; `SavedExchangeDeviceTest` 2/2 (hostile files through the real use case leave zero rows; valid GPX/KML/KMZ import once and export back only that owner's data).
- Real UI on device: `SavedPlacesDeviceTest` 3/3 (sheet favorite toggles and persists, another account does not see it; malicious file refused; valid import; identical file recognised; deleted id needs confirmation; account switch shows and exports only that owner's items) and the previously broken `MapScreenInstrumentedTest` 5/5 (see below).
- Real recorder service: `TripRecordingServiceDeviceTest` run 3 times, OK each time: no permission → refusal and no row; fixes before the explicit start record nothing; with a mock GPS provider only the good, ≥10 m spaced fixes are kept (poor-accuracy and 1.1 m fixes ignored); Stop saves a RECORDED_TRACK with the exact points; after stop further fixes are ignored and the service is gone; discard leaves no rows.
- Static: `:domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :data:lintDebug` BUILD SUCCESSFUL (domain 708, application 471, data 613 with 4 existing skips, app 548 tests, no failures); diff-check and secret scan clean.

## Defects found while proving this phase (fixed)

1. `TripRecordingService` cancelled its coroutine scope in `onDestroy` before the asynchronous save/discard finished, so an explicit discard (and sometimes save) could be lost. Teardown now waits for the storage work. Found by the discard device test.
2. `MapScreenInstrumentedTest` (MAP-19/20 UI proofs) had been failing since MAP-23 added a Hilt `guidance` default that a plain Compose host cannot supply. Tests now pass an explicit session-less `GuidanceViewModel` (`MapTestSupport.kt`); production code is unchanged. 5/5 pass again.

## Not delivered / boundaries

- **No cloud backup, signed private backup, cross-device restore or stale-restore proof.** The backend `favorites` dataset exists, but the Android cloud-sync client for it is not wired; creating that client or a trips dataset is outside this phase's targets. Cross-account restore is denied only in the sense that no restore path exists and imports always land in the current owner.
- The system file chooser (SAF) is not driven by automation; the bytes it reads/writes are produced and consumed by the proven use cases. The map sheet favorite is proven through `MapScreen` with an injected marker, not by tapping a real map pin.
- Recording while guidance runs is refused at recorder start; starting guidance while recording is not blocked (two location subscriptions would coexist) — to be decided with the product owner.
- No ARMv7/physical-phone run for this phase; emulator only.
