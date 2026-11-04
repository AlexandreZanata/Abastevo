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
