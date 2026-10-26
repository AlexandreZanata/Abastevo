# MAP-24 — Regional offline experience and update UX

Status: LOCAL_DONE
Validation: PASS (OFFLINE_GATE for the regional journey below; not production, store or physical-device certification)
Integration: INTEGRATION_PENDING

Parent: `f1e1d0db` (MAP-23 LOCAL_DONE). No payment, MAP-25–33, PAY, ESP, VPS/deploy or provider/live-source contact. Proof inputs are the admitted MAP-14 real MWM files (72,067,998 bytes) served by the loopback harness; no private route, location or user data.

## Product changes

- Downloads honour the existing Wi-Fi preference (`autoSyncOnWifi`): the unique WorkManager request requires an unmetered network, otherwise any connected network. The screen states which mode applies; retry/cancel use the same unique work.
- Inactive staged generations are listed and can be deleted from the UI. `deleteGeneration` refuses the active generation and any leased generation; the UI shows an explicit "in use" state instead of deleting.
- `GuidanceService` leases the active generation while a route is followed and releases it on stop/destroy. A lookup that races a stop cannot leave a lease behind (epoch check; found while reviewing the lease code).
- Activation, last-good fallback, atomic publication and restart recovery were already in `MapPackageRepository`; they are exercised here, not rewritten.
- No assertion was weakened: exact request counts are kept.

## Stabilising `MapOfflineDistributionInstrumentedTest`

Intermittent `SocketTimeoutException`/`unexpected end of stream` had three separate causes, none of them the adb tunnel itself:

1. The proof server answered HTTP/1.0 without `Connection: close`; OkHttp pooled the socket, the server closed it, and the next request of the same client (retries are off on purpose) read an empty response. The server now closes every response explicitly, and the test client disables idle pooling. `scripts/maps/native-proof-server.py` got the same header.
2. The "exact request count" read also counted the proof server's own `/proof/requests` query (off by one per read). Counting now includes only production `/v1/` paths.
3. The disk-full case re-imported the already-active generation, which is refused by design; it now advances the generation first.

Also found: the emulator had Wi-Fi disabled by an earlier airplane attempt (no `wlan0`). The suite now accepts `-e mapProofOrigin` (default `http://127.0.0.1:18766` via `adb reverse`; `http://10.0.2.2:18766` also works). Result: 4 tests × 3 runs on each origin = OK on `emulator-5554` (API 35, x86_64, 16 KB pages), traffic assertions unchanged. A fourth test was added: an interrupted update (transfer cut at 50,000 of 131,072 bytes) leaves the last-good generation complete/active/byte-identical, keeps exactly the 50,000-byte `.part`, and the retry resumes with a ranged request from byte 50,000, verifies the digest and publishes atomically. Harness: `scripts/maps/synthetic-distribution-server.py` (ephemeral Ed25519 key, Range, truncate/corrupt controls; needs the operator's existing Python `cryptography`).

## Real offline journey (OFFLINE_GATE)

`MapOfflineJourneyInstrumentedTest` + `scripts/maps/offline-journey-host.sh`, owned emulator only (`adb -s`; no physical phone, no `connectedCheck`):

1. Online phase: production Map tab → Downloads UI → candidate → download → published (real MWM, real signed manifest). Log: `map24/journey-online.log`.
2. Host: record proof-server `/v1/` request count (9), airplane mode on, Wi-Fi/data off (`wlan0` DOWN), `force-stop` (clean restart).
3. Offline phase in a fresh process, asserting before anything else that airplane is on, there is no active network and a public endpoint is unreachable, then: Map tab renders; viewport on Curitiba shows the downloaded map with ≥3 registered files; Downloads shows the published active generation; native street search and named-place search return real features; real route sheet uses a search result as origin and coordinates as destination and shows a preview; a Florianópolis destination is refused with the sheet's error plus a Downloads action (never a drivable success); engine guidance starts and a good synthetic fix advances progress; a street that exists only in the missing region returns no nearby feature. Log: `map24/journey-offline.log`.
4. Host compares the count again: **9 → 9** (the loopback tunnel stayed up on purpose, so any endpoint contact would have been counted). Repeated 5 times (including after a fresh clear + online phase): 5 OK.

Negative control: with the app data cleared and no downloaded map the same offline test fails at the "downloaded map at centre" step (`map24/journey-offline-negative-control.log`).

Limits: one first run after a fresh install + online phase ended with `Process crashed` (no FATAL/signal captured); five later runs, including the same sequence repeated, passed. Cause not isolated. The engine exposes feature name/coordinates only, so "POI detail" here is name+position selected as a route endpoint, not an opening-hours page. Price/overlay generations keep their own dates through the existing MAP-19/20 paths; this phase did not change or re-prove them. A complete cross-region route is the MAP-22 fixture proof; here the missing-neighbour refusal is what runs in the app UI.

## Static and unit checks

`./gradlew --offline --no-daemon --max-workers=1 :data:testDebugUnitTest --tests '*Map*' :app:testDebugUnitTest --tests '*Map*' --tests '*Guidance*' :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest :app:lintDebug :data:lintDebug`: BUILD SUCCESSFUL (data 98, app 71, no failures; one pre-existing skip). `scripts/maps` Python tests 13 OK, secret scan passed, `git diff --check` clean. No schema, migration, backend SQL or dependency change.
