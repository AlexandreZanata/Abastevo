# MAP-22 and MAP-23 — direct in-map navigation

## Behavior

The [MAP-22 and MAP-23 contract](../../../product/MAP_21_23_CONTRACT.md)
records the station-to-map handoff and the explicit navigation start. The
search arrow takes a unique routable result straight to the map preview. The
result list handles ambiguity. A station with a routable point preserves its
coordinates on entry. An address-only station opens offline search and waits
for a selected match. The preview uses the pinned CoMaps road graph and shows
its estimate and maneuvers on the map. Starting guidance requires precise
location and an available foreground notification. The foreground service
feeds GPS into native following, surfaces turn and progress information, and
uses the device's installed voice when available.

## Validation — 2026-11-04

- The debug APK and instrumentation APK built with JDK 21. Android debug lint
  and 102 focused unit tests passed across the app, map engine, domain and
  application modules. The app tests covered fresh, stale and missing origin
  fixes, superseded search, and preservation of active guidance.
- On a physical Android device with the offline Mato Grosso map, the search
  arrow and the charging card's route action both opened the Retrofit
  Energias — Centro destination inside the Campo Verde map. With no recent GPS
  fix, the destination remained available and the user could edit the origin.
  A synthetic origin entered in the editor produced a 1 km road route with
  visible blue geometry, turn arrows, a destination flag and a 2 minute
  estimate on the physical map.
- An Android 15 emulator with the same regional road graph built the route
  from injected GPS and started guidance explicitly. On the final APK the
  panel displayed the next turn at Avenida Brasil and a remaining 1 km. An
  earlier build of the same guidance flow changed the next turn to Avenida
  Mato Grosso and the remaining distance to 640 m after simulated movement.
  The emulator displayed the text-only fallback because it had no installed
  local voice and did not render the map background. The physical device
  supplied the geometry check. The instrumentation APK was compiled, and the
  device flows were exercised directly.
- The tested source is this commit on maintained `dev` from `3dd9c97e`. The
  change is included in protected PR #188. Runtime checks used debug builds.

## MAP-22 station origin regression — 2026-11-04

The earlier APK could display a native position and still wait for a GPS-only
route lookup. The route action now consumes a recent, accurate map snapshot
when precise permission is present. Otherwise it checks enabled providers'
recent device fixes and waits for the first suitable new fix, with a 15-second
limit and removal of listeners after success, timeout or cancellation. The
route preview still requires a separate action to start guidance.

On the updated physical APK, tapping **Rota e ir** on the Smile Posto card
in Sorriso and on the Auto Posto Cidade Alta card each calculated a native
route from the current position without entering an origin. On the Android 15
emulator, the Auto Posto Cidade Alta card waited
through a 250 m accuracy sample and calculated the route after a 10 m sample
from the fused test provider, with no injected GPS sample. A recent sample
from that provider also calculated the Smile Posto route in Sorriso from its
card, with a 2.2 km and 6 minute preview. No physical fix was recorded in the
evidence.

Debug build, lint and 62 tests passed, including regressions for the recent
map fix, fused cache fallback, poor fixes, timeout cleanup and coarse
permission. The tested source is this commit on maintained `dev` from
`abb2f461`, included in protected PR #188.

## MAP-22 route origin location policy regression

B-BR-MAP22-ORIGIN-01: An explicit route preview may request current fixes
through cancellable one-shot calls. An unsuitable fix can be retried at a
bounded cadence within the existing preview timeout. Accept the first fresh,
accurate fix and cancel pending calls when the preview completes or closes.
Continuous guidance remains in the private location foreground service,
with one high-accuracy provider at a time and the existing one-second request.

BUC-MAP22-ORIGIN-CI: Build a station preview with an empty location cache.
Reject an unsuitable fix, recover when an enabled provider returns a suitable
fix, and stop pending lookups on success, timeout or cancellation. Verify the
mobile guard still refuses continuous subscriptions in the preview and
unexpected providers in guidance.

GitHub Actions [Quick verification job 126670910848](https://github.com/AlexandreZanata/Abastevo/actions/runs/41857770150/job/126670910848?pr=188)
rejected the continuous subscription in the preview. The same mobile static
check failed locally before this change. The gate still rejects a planted
continuous subscription in preview, an unapproved guidance provider and a
shortened guidance interval. It verifies that guidance is a non-exported
location foreground service.

On the Android 15 emulator, the Abrantes preview in Sorriso waited after a
250 m accuracy sample and calculated a 2.8 km route after receiving a 10 m
fused sample, with no accurate GPS sample. The debug APK, lint, 19 focused JVM
tests, 25 gate-selection checks and local `make quick-verify` passed. This
commit records the tested source on `dev` from `c7158ef7` for PR #188.
