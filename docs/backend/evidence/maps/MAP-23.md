# MAP-23 — Explicit foreground guidance with local voice

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Rules: [MAP-21–23 contract](../../../product/MAP_21_23_CONTRACT.md).
Consumes MAP-22 `432557b2` plus pure boundary `b6a08e51`, vendored
explicit-guidance input `8fcc2b42` and implementation `31b31763`.
No payment, MAP-24–33, PAY, ESP, VPS/deploy or production certification.

## Product behavior

Following/progress uses the pinned core only: `Framework.nativeFollowRoute`,
`nativeGetRouteFollowingInfo`, `nativeGenerateNotifications` and the existing
native location feed through a same-package `NavigationFixBridge` (no SDK types
in the app). Launch binds to the exact preview request (`GuidancePolicy.sameRoute`);
an altered route refuses before any native following starts. Poor/stale fixes
(`NavigationFixPolicy`) never advance progress or speech. Reroute beyond 60 s
(`GuidancePolicy.REROUTE_TIMEOUT_NANOS`) surfaces recoverable failure, never false
success. `TrackRecorder` is explicitly stopped before following and before every
feed; navigation never saves a trace. `suspendMapReads`/`reloadMaps` refuse while
guiding, preserving the previous generation. `MapViewModel.dispose` and preview
`cancelRoute` never destroy a service-owned session; explicit `stopGuidance`
releases native following, routing points and notifications.

Foreground service `GuidanceService` (`foregroundServiceType=location`) owns the
single continuous GPS subscription; browsing never starts it. Start requires
explicit action with visible Activity, precise foreground location, enabled GPS
and visible notifications (API33 grant + enabled channel); planning stays
permission-free. Notification carries a Stop action. `START_NOT_STICKY`, no
boot/worker restart, no background location. `GuidanceRun` fences resources and
owner changes; logout/account switch synchronously stops fixes and resources.
Recovery stores only the owner-bound planning request in private `SharedPreferences`
(excluded from OS backup); process death shows explicit resume, never auto-starts GPS.

Voice uses Android `TextToSpeech` only during explicit guidance, with a
user-installed engine choice honored. Only installed local voices without
`isNetworkConnectionRequired` are accepted; network/download-required voices and
`setVoice` failures refuse and guidance stays textual. Navigation audio focus is
requested, lost on focus loss/calls, and abandoned on stop; stale engine
callbacks are fenced by generation. Native turn-text locale is mapped explicitly
(`pt`→`pt-BR`, `zh`→`zh-Hans`, `en/de/es/fr/ja/ru` direct, else null, fallback
`en` for native text only); no `Locale.language` is presumed a native voice code.

UI (`GuidanceControls` in `NativeRouteSheet`) shows start/stop, next maneuver,
distance/time, progress, recalculating, poor/lost GPS, voice-unavailable and
resume. All actions drive the service/core. Themes, 48 dp, TalkBack, narrow
screens, 200% text, safe areas and all eight locales preserved; lint
`MissingTranslation` intact.

## Defects found and fixed

1. Direct `Framework.nativeFollowRoute` without a rendered map crashed native
   (`SIGSEGV 0x10`) in the isolated engine test package. The adapter requires the
   real `MapHost` surface before following; the engine instrumented proof now
   renders via Compose `MapHost` and the app service proof launches the real host.
   No mock follow is accepted.
2. `GuidanceService` posted `engine.startGuidance` to `mainHandler` and waited on
   the Main thread, deadlocking `startForegroundService` (`ForegroundServiceDidNotStartInTime`).
   The service now calls the engine directly when already on Main. Proven by the
   passing foreground-service test on the phone.
3. `scripts/check-mobile.sh` forbade any `requestLocationUpdates` (P16 one-shot
   only), blocking the authorized MAP-23 foreground continuous GPS. The guard now
   allowlists only `GuidanceService.kt` (`GPS_PROVIDER, 1000L`, foreground service,
   no background permission) and still fails elsewhere. `make quick-verify` PASS.

## Executed evidence (owned phone, serial explicit)

- Phone: `VKCQGAUCGYAMR4W4`, `2311DRK48G`, Android 16 (SDK 36), `arm64-v8a`.
  No emulator was used for acceptance. All `adb` commands used `-s VKCQGAUCGYAMR4W4`.
- Pure: `:domain:test --tests NavigationFixTest/GuidancePolicyTest`,
  `:application:test --tests GuidanceRunTest` PASS (RED→GREEN preserved in
  `/tmp/map23-domain-red.log`, `/tmp/map23-runtime-red.log`, `/tmp/map23-runtime-green.log`).
- Engine JVM: `:maps:engine:testDebugUnitTest` PASS, including new
  `GuidanceLifecycleTest` (dispose preserves guiding session) and updated fakes.
- Merged AAR: `/tmp/map23-multi`, `LOCAL_SOURCE_ADMISSION_PASS`, upstream
  `4587b8ad4d12c9f59982709fdaf03cc925948827`, multi SHA
  `7ad828883c4aca8afc46601ea98ed340db5056cbc61df2e1896d8916d98d6837`
  (40,063,612 bytes), `arm64-v8a bb2aed...`, `armeabi-v7a 7d61b5...`,
  `classes.jar 071120e9...`, three patches applied, provision PASS.
- Phone fixtures (private runtime only, not shipped): `Brazil_Parana_East.mwm`
  (`d1e9b2ce...`, 10,058,648 bytes), `World.mwm`, `WorldCoasts.mwm` under
  `files/261004` via `run-as` stdin push; SHA verified.
- Phone native (explicit serial, no `connectedCheck` sweep):
  - `adb -s VKCQGAUCGYAMR4W4 shell am instrument -w -e class ...NativeRoutePlannerInstrumentedTest ...` → OK (5 tests).
  - `...GuidanceEngineInstrumentedTest` (Compose `MapHost` + real core) → OK (1 test):
    no trace, binding, progress, MWM refuse, stop.
  - `...GuidanceInstrumentedTest` → OK (3 tests): altered-route refusal without
    native follow, voice policy, real offline English speech when installed.
  - `...GuidanceServiceInstrumentedTest` (Compose host + FGS) → OK (1 test):
    service becomes active, stops via notification action, recovery cleared.
- Voice: Google TTS (`com.google.android.tts`) present; `LocalGuidanceVoice.start("en")`
  + `speak` PASS offline on the phone; unknown language refuses fast, textual
  maneuvers preserved. No cloud TTS dependency added.
- Regression: `:domain:test`, `:application:test`,
  `:maps:engine:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:lintDebug`
  PASS; `git diff --check` PASS; secret surface reviewed (synthetic route points,
  no credentials/contributor GPS).
- Release: `:app:assembleRelease -PcomapsSdkDir=/tmp/map23-multi` + 
  `:app:verifyReleaseApkSize` PASS — `app-release-unsigned.apk` 78,204,622 bytes
  (74.58 MiB) under 80 MiB, both ABIs present with admitted `.so` SHAs,
  16 KB LOAD alignment preserved by the admitted input.

## Remaining boundaries

MAP-24–33, production distribution, broad performance/ABI/device certification,
charging live coverage and commercial delivery are not completed. Staging,
VPS, wiki and merge are separate. New protected PR remains pending until batch end.
Process-death resume was proven as explicit-only (no auto GPS); full power-cycle
and long wrong-turn (>60 s) field timing remain OWED beyond the synthetic
reroute unit bound. Two-account/device manual and rendered accessibility beyond
the automated Compose checks remain OWED.

## 2026-10-26 current-head revalidation (phone, explicit serial)

The current batch re-ran the guidance surface on the authorized physical phone
`VKCQGAUCGYAMR4W4` (Android 16/API 36, arm64-v8a, 4 KB pages), with explicit
`adb -s` serial and the admitted Parana/East fixture. No emulator result is
substituted for this phone run, and no phone result is used as 16 KB runtime
proof.

- `GuidanceInstrumentedTest` 3/3 OK (altered route refused, poor fixes rejected,
  locale/voice policy, offline English voice path).
- `GuidanceServiceInstrumentedTest` 1/1 OK (real rendered host, foreground
  service active, stop via notification action, recovery cleared).
- `GuidanceEngineInstrumentedTest` 1/1 OK. Added a fresh 1 Hz synthetic replay
  beside a real native route: the core reported `RouteRebuilding` and the
  timeout path returned `rerouteRecoverable = true` at simulated index 80
  (more than 60 s after the off-route state started). A single 61-second-old fix
  would be rejected by `NavigationFixPolicy`; the replay therefore uses fresh
  fixes at one-second simulated intervals. No trace was recorded.
- The failure/stop path is cleaned up in `finally`; the earlier failing attempt
  that left a guidance session open is not counted, and its process crash is
  disclosed as test-harness instability rather than a passing result.

Still OWED: real long wrong-turn field timing beyond the synthetic replay,
physical 16 KB runtime (the phone has 4 KB pages), two-account/device manual
traversal and rendered TalkBack beyond the Compose semantics tests.
