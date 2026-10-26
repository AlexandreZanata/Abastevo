# MAP-18 — Markers, clustering and selection

Current assessment: [2026-10-25 remediation](MAP-15_20_REMEDIATION_20261025.md) supersedes earlier integration/acceptance claims. Historical evidence below is retained.

Status: PARTIAL / INTEGRATION_PENDING
Validation: scoped checks PASS; local real-data native pins PASS (2026-10-25);
Abastevo-owned combined/electric native artwork remains PARTIAL
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
- 2026-10-25: KML station labels use the user-facing station name and clusters
  use their count; the former `ABASTEVO:<uuid>`/`ABASTEVO_CLUSTER:<id>` labels
  were removed. Selection now maps the exact rendered point back to the
  canonical UUID, so identity is not exposed as map text. See the
  [local MAP-20 acceptance](MAP-20_MINIMUM_LOCAL_20261025.md).

### Business Rules & Use Cases (Review Remediation)

- **B-BR-MAP-CLUSTER-01 (Cluster Member Reachability)**: Tapping a multi-station cluster (`MapOverlaySelection.Cluster` or `onClusterExpanded(clusterId)`) must never silently pick only the first station. It must expose all cluster members in an accessible chooser (48 dp targets, TalkBack descriptions) so the 1st, 2nd, and last members are individually selectable, and reconcile selection across overlay generation updates.
- **B-BR-MAP-UNPINNED-SELECT-01 (Coordinate-Independent UUID Selection)**: `MapDiscoveryViewModel` must index all discoverable stations by canonical UUID (`selectableByUuid`), including `UNKNOWN`, `CITY_CENTROID`, and null-coordinate records. Selecting an unpinned or coordinate-less station from `MapOverlayList` opens the bottom sheet and station profile normally, while disabling coordinate routing (`canRouteByCoordinates == false`) with an explicit user-facing reason—never routing to `0.0, 0.0` or a city centroid.
- **B-BR-MAP-KML-COMBINED-01 (Distinct Combined KML Style)**: Single-station placemarks in `buildOverlayKml` must distinguish fuel-only (`orange` / `Gas`), charging-only (`teal` / `ChargingStation`), and combined fuel+charging (`purple` / `FuelAndCharging` description + distinct style) so combined stations are not collapsed into charging-only in KML output.
- **BUC-MAP-18-01**: User explores a dense area or list -> expands a multi-station cluster or selects an unpinned station -> chooses any member -> views its bottom sheet and profile, with coordinate routing enabled only for `REVIEWED` coordinates.

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
