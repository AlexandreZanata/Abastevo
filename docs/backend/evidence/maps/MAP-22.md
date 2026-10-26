# MAP-22 — Native offline route planning

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Rules: [MAP-21–23 contract](../../../product/MAP_21_23_CONTRACT.md).
Consumes MAP-21 `2f10ad33` and the inherited local minimum map. New source/runtime
work is local; no production route/download origin or readiness is inferred.

## Product behavior

The real Map tab and station sheet now open a native planner. Origin/destination
and ordered stops accept offline SDK search, a manually chosen map center,
coordinates, a reviewed station reference or an explicit cancellable one-shot
current fix. No permission/GPS is needed for manual planning. Freshness/accuracy
refusal of the one-shot fix leaves map/manual selection available; no ongoing
subscription is created. Unknown entrances remain explicitly approximate.

The domain/port limit matches pinned `RoutePointsLayout` (100 intermediate stops).
Invalid/nonfinite/duplicate points refuse before JNI. The real Vehicle core
returns geometry, maneuver text, distance/time estimates and road warnings.
Toll/motorway/ferry/unpaved avoidance maps to actual SDK options; other inherited
options are cleared. No traffic, fabricated alternatives, straight-line route or
external geo intent represents native success. Ordered stops can be moved/removed.
Planning inputs and avoidance settings survive recreation; geometry must rebuild.

Generation fences and bounded deadlines cancel obsolete route/search work.
Missing native neighbor dependencies replace any provisional preview with an
explicit failure, names and the existing downloads entry. UI text covers all eight
locales, large/scrollable controls, accessible stop movement and status-bar insets.
Renderer completion is distinct from engine initialization; the host reapplies
its viewport on actual graphics readiness.

## Defects found and fixed

1. The SDK Java controller invokes `onBuiltRoute` for NEED_MORE_MAPS. The adapter
   owns the pinned native listener directly and accepts only NO_ERROR/HAS_WARNINGS
   with a valid built route/geometry. It preserves later dependency failures.
2. A new adapter initially passed 0 to junction interpolation; the real device
   test exhausted memory. The fixed adapter uses the SDK's exact +infinity
   non-interpolation sentinel. The actual native 1.7 km fixture must contain more
   than two and fewer than 5,000 original junctions. The failed run is not accepted.
3. Actual native INFO diagnostics exposed route checkpoints. Rebuilt both release
   ABIs with [private-diagnostics.patch](../../../../maps/engine/upstream/private-diagnostics.patch),
   suppressing native/Java payloads and native assertion fallback contents.
   No exception or user-location text enters SDK logcat/file sinks. This does not
   disable checks/aborts or replace route error codes. The host does not log fixes.
4. Visual checking initially captured a blank/animation frame and then a saved
   world camera. The UI fixture now seeds its own private camera, waits for real
   street-scale native readiness and requires rendered map content; it does not
   weaken the preview assertion or substitute a fake surface.

## Executed evidence

- Pure contract RED (permissive stub failed invalid/duplicate/limit tests) -> GREEN.
- `:domain:test --tests '*NativeRouteTest'`, `:maps:engine:testDebugUnitTest`,
  `:app:testDebugUnitTest`, `:app:lintDebug`, debug app/instrumentation builds PASS.
  ViewModel regressions prove ordered stops/options without GPS, stale callbacks,
  dependency failures revoking previews, cancellation, timeout and input restore.
- Owned API35 `anpfuel-16k`, 16,384-byte pages, read-only/no-snapshot, ARM64 native
  bridge/GLES3, airplane mode/Wi-Fi/data off: five real native adapter tests + the
  production Map-tab UI journey PASS. Covers actual offline street search,
  multi-stop route/maneuvers/options, no-map refusal, cancellation and diagnostics.
  OSM source way 125539246 (Rua Barão do Serro Azul, oneway=yes) requires a reverse
  detour rather than driving the wrong way. Its admitted source.o5m SHA-256 is
  `6a4304dfcf397a454ee1590199fab62c470d62486e9f2962af6f06fbc502b137`.
- [Rendered actual route](MAP-22/native-route.png), after native camera settles
  at Curitiba street scale. The no-package banner is correct for this controlled
  fixture: maps were preseeded privately for a runtime proof, not admitted into
  the app's signed-distribution Room registry. Production distribution is unchanged.
- Independent complete-neighbor profile: official series 2026.06.28 / data version
  261004 East + West Paraná, sizes and SHA-1 checked against countries.txt at
  source 4587b8ad; SHA-256 and URLs in [fixture receipt](MAP-22/neighbor-fixtures.json).
  The actual offline Curitiba -> Guarapuava route crosses both regional files.
  After cancelling and suspending readers, withdrawing West refuses and names
  Parana_West; restoration succeeds in `finally`. PASS. Files are not shipped or
  committed; no upstream product-CDN entitlement is inferred. The original
  generated Curitiba fixture was restored only after stopping all app readers.
- Source builder/auditor PASS separately for ARM64 and ARMv7; merged AAR and all
  215 source components admitted, 193 pinned submodules clean, 0 unresolved licenses/
  endpoints. Resources/notice gates and original source pin remain unchanged.
  [Exact two-ABI receipt](MAP-22/native-build.json); updated vendored input/source
  offer/SBOM and deterministic builder apply both recorded patches. 16 KB ARM64
  LOAD alignment preserved. Thirteen provisioning-tool tests PASS. ARM32 runtime
  regression and clean-source comparison are separate evidence; neither is
  silently inherited from the prior artifact's MAP-14 runtime matrix.
- `bash -n`, diff-check and secret surface reviewed: synthetic route points/test
  proofs only, no private credentials/contributor traces. No new external library.

Logs: `/tmp/map22-{domain-red,engine,app,private-runtime,controlled-camera,
  cross-runtime,exit,builder-tests}.log` plus `/tmp/map22-arm32-runtime.log`.
Posterior ARMv7 runtime: the same five native adapter tests PASS through the
ARMv7 bridge on the isolated API30 emulator (`codex-map22-api30`); this does
not prove all ARM32 graphics paths nor a clean-source comparison. The earlier
interpolation OOM and UI camera failures remain diagnostic history; only the
repaired matrix is accepted.

## Remaining boundaries

MAP-23 owns explicit foreground GPS/following/rerouting and offline local voice.
Existing SDK automatic TTS initialization will be replaced there with explicit
host ownership. No guided navigation claim is made by this checkpoint.
MAP-24–33, production distribution, broad performance/ABI/device certification,
charging live coverage and commercial delivery are not completed by route preview.
