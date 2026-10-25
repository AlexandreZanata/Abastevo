# MAP-18 — Markers, clustering and selection

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM clustering + discovery, real-phone list)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `3ccb6988` plus MAP-18 changes.
Date: 2026-10-25. Prices and search remain MAP-19/20.

## Behavior frozen

- Pins use canonical UUIDs as stable identity; proximity never merges records.
  `prepareOverlay` deduplicates by lowercased UUID, merges duplicate service
  flags (fuel/charging) keeping the reviewed location, and clusters
  deterministically within 60 m. Cluster IDs are generation-scoped
  (`c<gen>-<index>`); the UI never exposes `ABASTEVO_CLUSTER` or raw titles.
- B-BR-MAP-01: only `REVIEWED` becomes an exact pin. `UNKNOWN` and
  `CITY_CENTROID` never reach the native KML; they stay in the accessible
  list with explicit warnings. `buildOverlayKml` contains only pins.
- Fuel, charging and combined sites are distinct: fuel pump vs bolt/plug
  pictograms side-by-side for combined, plus text labels (never color alone).
  Reference OSM POIs stay distinct: `overlaySelectionFor` returns null for
  generic OSM titles, never inventing an Abastevo identity.
- Viewport queries are bounded (500–50 000 m, limit 100), debounced 300 ms,
  generation-checked; stale responses are discarded. Selection (UUID) is
  preserved across generations when the UUID survives; cluster tap expands
  to members.
- Accessible equivalent `MapOverlayList` uses the admitted pictograms
  (`ic_map_station_combustion/electric`, 24 dp decorative), 48 dp minimum
  targets, TalkBack content descriptions and Material light/dark themes.
  No overlay update rebuilds the routing graph.
- No `ABASTEVO_CLUSTER` in UI, no unknown/city-centroid pins, no native
  atlas/style changes (SDK `Gas`/`ChargingStation` symbols retained, so no
  admission update was needed).

## Implementation

- `maps/engine/.../MapOverlay.kt`: `prepareOverlay` splits pinnable vs
  `unpinned`, `overlayListItems(..., unpinned)` keeps list-complete.
- `maps/engine/.../MapOverlayList.kt` (reused): pictograms, 48dp, TalkBack.
- `app/ui/map/`: `MapDiscoveryViewModel` (viewport + charging snapshot,
  debounce, stale discard, selection), `ChargingMarkersSource` +
  `MapDiscoveryModule` (offline snapshot, fail-open to fuel), `MapScreen`
  list toggle.
- `maps/engine/.../MapOverlay.kt`: `prepareOverlay` made public for the app
  port (no native change).

## Verification executed

JVM (`:maps:engine:testDebugUnitTest`, 27 cases; `:app:testDebugUnitTest`):

- `OverlayKmlTest` (9): canonical KML, duplicate merge, co-located distinct
  stay separate, invalid rejected, cluster icon, OSM-vs-Abastevo disambiguation.
- `OverlayPinEligibilityTest` (2): unknown/city-centroid list-only with
  warnings, UUID stability across source changes.
- `MapDiscoveryViewModelTest` (6): bounded debounced viewport with stale
  discard; unknown/city-centroid list-only; deterministic clusters with
  preserved selection; cluster expansion without technical IDs; fuel/
  charging/combined distinguishable; dense 50-marker deterministic clustering.

Phone (`:app:connectedDebugAndroidTest`, 2311DRK48G):

- `MapScreenInstrumentedTest` retains shell proof with the real list toggle.
- Pictogram light/dark inspection remains `preview.png/svg`; 20/24/48/128 px
  exports are the admitted Android PNGs. No new native artwork was shipped.

## Limits

- Charging is offline snapshot only; live availability is never claimed.
- Real `nearby` wire proof stays MAP-20 (backend-local + device list).
- Prices, sheet and search/filters remain MAP-19/20.
