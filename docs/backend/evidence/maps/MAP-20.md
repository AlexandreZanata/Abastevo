# MAP-20 — Discovery, accessibility and minimum gate

Current assessment: [local real-data acceptance](MAP-20_MINIMUM_LOCAL_20261025.md) (2026-10-25) supersedes the earlier “no real pins/prices” blocker for **local proof only**. Staging/production data and distribution remain pending. Historical evidence below is retained.

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: scoped local checks PASS, including real ANP/OSM → PostGIS → API → Kotlin → native pins → quick price → profile and offline restart
Integration: INTEGRATION_PENDING
Gate: MINIMUM_MAP_GATE LOCAL_PASS; STAGING PENDING; PRODUCTION PENDING

Tested tree: `codex/map-ev-implementation` at MAP-18 plus MAP-19/20 changes.
Date: 2026-10-25.

## Behavior frozen

- Search by area/city/name/address: `onViewport` (100–15 000 m, limit 100)
  + `onSearch(municipalityCode, query)` (2–100 chars, limit 1–100, cursor-bound),
  both debounced 300 ms, generation-checked, stale discarded. Unknown metadata
  (null coords, unknown quality/power/tariff) is shown with warnings, never
  excluded.
- Filters: ANP/Community source (explicit, invalidates older responses),
  fuel/charging service (local, at least one stays visible), product/connector/
  power scoping owns its follow-up (unknown power/tariff shown, not excluded).
  Rapid source/filter switches never paint old responses.
- Accessible equivalent: `MapOverlayList` with admitted pictograms (fuel pump
  + bolt/plug, side-by-side for combined), 48 dp minimum targets, TalkBack
  content descriptions, text labels (never color alone), Material light/dark,
  safe-area padding, 200% font and narrow-screen flexible layouts. Focus
  returns to the list toggle; controls have descriptions; OSM attribution stays
  visible.
- Eight locales for all map strings (`nav_map`, `map_title/source/search/
  filters/downloads/loading/unavailable/retry/no_package/download/attribution/
  search_hint/filter_service/fuel/charging/city/apply/no_results`).
- Flow: open Mapa → choose source → see real staging stations in list →
  select list/marker → sheet shows explicit absence (no invented price) with
  quality/location → open full profile (`map/station/...` keeps MAP selected)
  → explicit external `geo:` route (never counted as native guidance; native
  `buildCarRoute` preview owns MAP-22).

### Business Rules & Use Cases (Review Remediation)

- **B-BR-MAP-DISCOVERY-01 (Dynamic Viewport, City Selection & Generation Invalidation)**:
  1. Camera/viewport changes from `MapHost` / `MapViewModel` drive bounded debounced `onViewport` queries without hardcoded coordinates, and city selection uses the configured/selected IBGE municipality code (with city switching in UI) without requiring GPS.
  2. `ChargingMarkersSource` pages through the active offline snapshot generation with region/query/filter matching rather than silently truncating at the first 100 global rows, rethrows `CancellationException`, and surfaces snapshot errors/emptiness honestly.
  3. Generation counter (`++generation`) and `fetchJob?.cancel()` execute immediately before debounce in `onViewport`, `onSearch` (including when clearing or shortening queries to `< 2` characters), and filter changes so stale in-flight responses can never overwrite newer state.
- **B-BR-MAP-FILTERS-01 (Contract-Accurate Fuel & EV Filters)**: `MapFiltersDialog` and `MapDiscoveryViewModel` support filtering by price source (`ANP` / `COMMUNITY`), service (`fuel` / `charging`), fuel product (`FuelProduct`), EV connector (`ChargingConnector`), current (`ChargingCurrent` AC/DC), access (`ChargingAccess`), minimum known power (`kW`, keeping unknown power visible), and survey period; server station directory list/nearby endpoints do not filter by fuel price on the wire, so fuel product and survey period scope the active sheet/profile price query while EV filters scope charging markers/list items directly.
- **B-BR-MAP-I18N-A11Y-01 (8-Locale Completeness, Narrow Layout & Route Failure Feedback)**: All user-facing map strings (including `MapBottomSheet`, `MapDownloadsScreen`, `MapFiltersDialog`, `MapHost`, location quality labels, and route errors) are localized across all 8 locales (`en`, `de`, `es`, `fr`, `ja`, `pt-rBR`, `ru`, `zh-rCN`), narrow-screen controls use flexible widths without clipping, and failed external route intents display an actionable message.
- **BUC-MAP-20-01**: User pans the map or selects a city/search query -> filters by fuel product or EV connector/current/power/access -> selects a station from a cluster or accessible list -> views localized sheet/profile or triggers external route with honest error handling.

## Implementation

- `MapDiscoveryViewModel`: dynamic viewport + city selection (`DEFAULT_MAP_CITIES` + `MunicipalityCatalogRepository`/`UserPreferencesRepository`) + search + extended filters (`showFuel`, `showCharging`, `fuelProduct`, `connector`, `current`, `access`, `minPowerKw`, `surveyPeriod`), `selectableByUuid` indexing pinned, unpinned (`UNKNOWN`, `CITY_CENTROID`), and null-coordinate stations, accessible cluster expansion (`expandedClusterId`, `selectedMembers`), and pre-debounce generation invalidation (`++generation`, `fetchJob?.cancel()`).
- `MapScreen`: ANP/Community chips (48 dp), active city chip (`map-city-chip`), search field (48 dp, clears/shortens immediately invalidating in-flight search), extended filter dialog (`MapFiltersDialog`), accessible cluster expansion controls (`map-cluster-expand-*`) and member chooser (`map-cluster-chooser`), `MapOverlayList` toggle (48 dp), attribution, loading/error/no-package, and safe route guard (`canRouteByCoordinates`).
- `MapSheetViewModel` + `MapBottomSheet` + `RealSheetPriceSource`: real production price binding in `MapSheetModule` (ANP station detail + Community city feed/detail + EV community reports), pre-branch generation invalidation fixing the ANP EV source-switch race (`MapReviewRegressionTest`), localized strings across 8 locales, and explicit disabled route notice (`map-sheet-route-unavailable`) for stations without `REVIEWED` coordinates.
- `ChargingMarkersSource` + `MapDiscoveryModule`: paginated EE-18 snapshot scanning across pages of 100 (eliminating silent 100-item global truncation), region/query/connector/current/access/minPowerKw filtering (preserving unknown power), and `CancellationException` rethrow.
- `MapServerDecodeTest`: staging Go JSON decodes + real localhost HTTP with Go bytes (resolved across root and `:data` working directories).
- `settings.gradle.kts` + `maps/engine/build.gradle.kts`: unconditional `:maps:engine` inclusion and configurable `comapsSdkDir` resolution with SHA-256 verification of `classes.jar` and both ARM `liborganicmaps.so` binaries.

## Verification executed

JVM (`:domain:test`, `:application:test`, `:maps:engine:testDebugUnitTest`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:lintDebug`, `:app:assembleRelease`, all green):

- `MapDiscoveryViewModelTest` (12): bounded debounced viewport + stale discard;
  unknown/city-centroid list-only; deterministic clusters + preserved selection;
  cluster expansion without technical IDs; fuel/charging/combined distinct;
  dense 50-marker deterministic; search by city/name bounded + short-query no-wire;
  service filters keep unknown visible; unpinned and coordinate-less stations
  selectable by UUID with `canRouteByCoordinates == false`; cluster expansion
  reaches 2nd and last member and reconciles across generations; clearing/shortening
  search during debounce or in-flight cancels prior query; `ChargingMarkersSource`
  paginates beyond first 100 rows and preserves unknown power.
- `MapSheetViewModelTest` (12) + `MapReviewRegressionTest` (1): exact product/unit/
  amount/date; explicit absence without fallback; rapid switches discard stale;
  EV-in-ANP explains + no lookup; real `RealSheetPriceSource` ANP/Community/EV
  price resolution + error/absence distinction; permanent regression proof for
  ANP fuel → ANP charger race.
- `OverlayKmlTest`: distinct `#placemark-orange` (fuel), `#placemark-teal`
  (charging), and `#placemark-purple` (combined) styles.
- `MapServerDecodeTest` (2): staging `GET /v1/stations` Go JSON decodes with
  canonical UUIDs/CNPJ leading zeros; same Go bytes over real localhost HTTP
  decode identically (production codec, no invented price).
- `MapDownloadsViewModelTest`, `MapScreenViewModelTest`, `MapNavigationTest`,
  `NavigationTabTest` all PASS.

Phone (2311DRK48G / `VKCQGAUCGYAMR4W4`, Android 16):

- `MapRouteInstrumentedTest`: real `MainActivity` + Hilt + native `ComapsMapEngine`
  renders MAP tab, survives tab switch, rotation, and activity recreation with
  a single engine instance.
- `MapNativeLayoutInstrumentedTest`: real signed download → `<formatVersion>/<file>.mwm`
  activation → native `FindAllLocalMapsAndCleanup` registration and concurrent
  reader retention.
- `MapScreenInstrumentedTest` & `MapOverlayListAccessibilityTest`: source switch,
  cluster expansion (2nd/last member), unpinned selection with route disabled,
  and light/dark TalkBack descriptions.

Backend (staging `https://teste.abastevo.com.br`, 2026-10-25):

- `GET /v1/stations?limit=5` → 200 with 5 real UUIDs/CNPJs (e.g.
  `0002ead6-…/CENTRO AUTOMOTIVO NEBBIOLO`, all `location_quality:unknown`,
  `coordinates:null`) — decoded by `MapServerDecodeTest`.
- `GET /v1/stations/nearby?lat=-25.42&lon=-49.27&radius_m=10000` → 200 empty;
  Brasilia and charging `places?profile=ocm-br-…` also empty. No REVIEWED
  coordinates and no station prices in staging for pins/prices.

Release APK (with accepted engine + Monocypher + map UI):

- `app-release-unsigned.apk` 78,103,646 bytes / 74.49 MiB, under the 80 MiB
  ceiling. Both ARM ABIs (`arm64-v8a`, `armeabi-v7a`) contain uncompressed
  16 KB-aligned (`0x4000`) `liborganicmaps.so` + `libmonocypher_ed25519.so`.

## Minimum-gate assessment (honest, no fake PASS)

Proven in app (phone + JVM + real staging bytes):

- Open Mapa → real Hilt + native `ComapsMapEngine` initializes via `MapViewModel`
  → choose ANP/Community or city → see real staging stations in accessible list
  (UUIDs/names preserved, unknown warnings, no centroid pins).
- Select any station (pinned, cluster 2nd/last member, `UNKNOWN`, `CITY_CENTROID`,
  or null-coordinate) → sheet loads via real `RealSheetPriceSource` without
  source-switch races, disables coordinate routing when coordinates are not
  `REVIEWED`, and opens full station profile (`map/station/<uuid>`).

Blocked by external data/operations (reproduced above, not a code waiver):

- No REVIEWED coordinates in staging (all `unknown` + `null`), so native pins
  cannot appear with live staging data yet.
- No station-specific ANP/community prices or EV observations populated in live
  staging yet.
- No signed `.mwm` distribution host/public key configured in runtime environment
  yet (`MapDownloadsScreen` explicitly surfaces `config-missing` until configured).
- Native route preview (`buildCarRoute`), offline search, and MAP-21–33 remain
  scoped to the next phases.

Gate: **LOCAL_PASS / STAGING_PENDING / INTEGRATION_PENDING**. See the
[local real-data record](MAP-20_MINIMUM_LOCAL_20261025.md) for the executed
chain, 62 reviewed pins, 33 exact prices, native pin/sheet/profile screenshots,
offline process-restart cache and APK hashes. Do not merge/publish/certify as a
production gate. Next: staging distribution/data configuration and MAP-21–33.

## Completion behavior — 2026-10-25

B-BR-MAP-CITY-02: the city chip reuses the national IBGE picker and returns
to MAP. A city without a reviewed point uses its scoped Directory list; it
never borrows the previous city's coordinates or fabricates a centroid pin.
B-BR-MAP-PERIOD-02: ANP map prices reuse the existing survey-week picker.
Unavailable opening-hours metadata is explicitly unknown; no open-now claim
or synthetic opening-hours filter is introduced. Native combined artwork
remains an explicit product criterion until its own glyph is admitted.
