# MAP-29 — Measured performance, compatibility and operational budgets

Phase commit: `1a6a1add` (this evidence commit; docs-only revisions after it do not change code).

Status: LOCAL_DONE
Validation: PASS for the declared local proxy budgets; **PERFORMANCE_GATE PARTIAL**
Integration: INTEGRATION_PENDING
Gate: PERFORMANCE_GATE PARTIAL — the local 7-item proxy matrix passes, but the full MAP-29
matrix still has owed external/device/DB/frame/network items listed below. This is not a
national capacity, commercial SLA, physical-battery or release certification.

Contract written before measurement: [MAP_29_PERFORMANCE_CONTRACT](../../../product/MAP_29_PERFORMANCE_CONTRACT.md).
Raw sanitized samples: [raw-perf.txt](map29/raw-perf.txt). Static receipt: [static.txt](map29/static.txt).
Reproduction recipe: `scripts/maps/measure-map29.sh`.

## Environment

- Owned `emulator-5554`: Android 15/API 35, fingerprint
  `google/sdk_gphone16k_x86_64/emu64xa16k:15/AE3A.240806.043/12960925:userdebug/dev-keys`,
  kernel page size 16,384, app ABI executed `arm64-v8a` through the host ARM translation,
  native GLES path, debug app. Host Linux x86_64; JDK 21 (Temurin 21+35); Gradle 8.10.2.
- Admitted regional fixture in app-private files: `Brazil_Parana_East.mwm`
  SHA-256 `d1e9b2ce81cc10335b6b082a64d78a8241390f42c262e2832f400c981b200a3e`,
  `World.mwm` `77a35c16…`, `WorldCoasts.mwm` `d42a71b1…`. Synthetic Curitiba route
  endpoints; no real GPS/route trace in the receipt.
- Release static candidate: universal unsigned APK `78,396,538 bytes` = **74.76 MiB**
  (78.40 MB), SHA-256
  `feadd9f34d60c26b57a2025e96a1b799c04b52c90da9d47c9784a430363536e0`. Existing 80 MiB
  ceiling unchanged. Debug runtime APK SHA-256 `18f283c6…`; test APK `bc04e1ed…`.
- Source measured: parent `d766dbb4` plus the MAP-27–29 code. The exact tree is the phase
  commit carrying this file; evidence/docs were added after the measurements and do not change
  code. No production gate is inferred from this local tree.

## Local proxy matrix (3 samples)

| Metric | Samples | Declared local budget | Result |
|---|---|---|---|
| Engine init to native readiness | 1149, 752, 754 ms | ≤ 15,000 ms | PASS |
| Real 1,800 m route build | 410/86/75; 347/87/72; 217/78/74 ms | ≤ 5,000 ms each | PASS |
| Process PSS after routes | 351,481; 350,505; 349,163 KiB | ≤ 524,288 KiB (512 MiB) | PASS |
| Native route cancel | 0, 0, 0 ms; 0 callbacks after cancel | ≤ 1,000 ms, no stale result | PASS |
| App cold launch `am start -W` | 1218, 1384, 1242 ms (one extra warm sample discarded) | ≤ 4,000 ms | PASS |
| Release universal APK | 78,396,538 bytes / 74.76 MiB | ≤ 80 MiB | PASS |
| ABIs + 16 KB packaging | arm64-v8a + armeabi-v7a; every packaged `.so` LOAD `0x4000`; `zipalign -c -P 16 4` PASS; no `.mwm` assets; manifest minSdk 26 | supported ABIs, 16 KB, no test maps | PASS (static) |

Notes:
- PSS is the `Debug.MemoryInfo.totalPss` of the instrumentation/app process; it includes test
  and engine overhead. It is an emulator proxy, not a low-end physical budget.
- Engine init is readiness, not "time to useful map content"; content/frame pacing remains
  OWED. The MAP-28 UI test separately waits for rendering and passes functionally.
- One earlier perf attempt crashed in the emulator framework after logging init/routes/PSS but
  before the cancellation step (`MonitoringInstrumentation.queueIdle`/JIT SIGSEGV). The final
  direct-cancel runs above all passed. The failed run is not counted.
- The cold launch is app launch on Home, not map content. The extra WARM sample is retained in
  `raw-perf.txt` and explicitly discarded; the three COLD samples are the evidence.
- Static ELF alignment is not runtime execution. API 30/API 35 results are not API 26 proof.

## Physical reference profile (4 KB, informational)

The same `MapPerformanceInstrumentedTest` was run 3 times on the authorized phone
`VKCQGAUCGYAMR4W4` (POCO/duchamp, Android 16/API 36, arm64-v8a, PAGE_SIZE 4096), with the
admitted regional fixture and no network provider:

| Metric | Samples | Same local budget | Result |
|---|---|---|---|
| Engine init | 439, 354, 363 ms | ≤ 15,000 ms | PASS (proxy) |
| Real 1,800 m route | 125/46/51; 82/55/57; 85/59/64 ms | ≤ 5,000 ms | PASS |
| Process PSS after routes | 276,902; 275,743; 277,633 KiB | ≤ 512 MiB | PASS (proxy) |
| Cancel | 0, 0, 0 ms; 0 callbacks | ≤ 1,000 ms | PASS |
| Cold app launch | 1250, 1210, 1125 ms | ≤ 4,000 ms | PASS |

Raw: [raw-phone-perf.txt](map29/raw-phone-perf.txt). This is a **4 KB physical profile**, not a
16 KB runtime proof; the emulator S2 remains the 16 KB profile. It also does not establish
frame pacing, battery/GPS autonomy or low-end/national coverage.

## Functional regressions used

- `EvTripRealEngineInstrumentedTest` OK (3.037 s), `EvTripUiInstrumentedTest` OK (20.339 s),
  `EvTripSectionAccessibilityTest` OK (2.133 s) on the reference emulator. These prove the
  MAP-27/28 real-core/UI journey; they do not prove frame pacing, battery or national coverage.
- `:domain:test :application:test`, targeted data/app map/saved/ledger/EV tests, and
  `:app:lintDebug :data:lintDebug :maps:engine:lintDebug` pass on the measured source.

## Owed / not claimed

- Frame time/pacing/jank for Compose **and** the native SurfaceView, time to useful map
  content, repeated rotation/navigation/process-death leak growth. MAP-14's historical
  21–27 FPS and SurfaceFlinger queue observations are context only.
- Network bytes/download resume/304/cancel behavior on the current head. MAP-24's 9 → 9
  regional proof remains historical evidence, not this run's measurement.
- PostGIS density/concurrency, cursor pagination, EXPLAIN/index checks, job backpressure and
  regional generation resource ceilings. No SQL/job/schema change was made in this batch and
  no disposable PostGIS service was used.
- API 26 runtime and clean ARMv7 JNI runtime on this head. Static packaging is not runtime
  proof; the only attempted ARMv7 profile died with the emulator during a UI run.
- Physical GPS/CPU/battery autonomy, low-end native hardware, dense/sparse national regions,
  staging CDN behavior, commercial SLA and release certification.
- Release **runtime** validation; only the release APK static/size/alignment receipt is
  claimed. Debug runtime results do not transfer to release.

`PERFORMANCE_GATE` therefore remains **PARTIAL**, not PASS. No waiver is recorded.
