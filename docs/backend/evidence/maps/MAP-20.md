# MAP-20 — Discovery, accessibility and minimum gate

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM search/filters/decode + real-phone list/sheet/accessibility)
Integration: INTEGRATION_PENDING
Gate: MINIMUM_MAP_GATE PARTIAL (real list/profile/route proven; real pins/prices blocked by staging data, documented below — no fake PASS)

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

- `MapDiscoveryViewModel`: viewport + search + service filter, `lastMarkers`
  + `lastNullStations` (null-coords stay list-only), `prepareOverlay` split,
  `overlayListItems(..., unpinned)` + null-coords items.
- `MapScreen`: ANP/Community chips (48 dp), search field (48 dp, Curitiba
  `4106902` default for gate), service filter dialog (48 dp checkboxes),
  `MapOverlayList` toggle (48 dp), attribution, loading/error/no-package.
- `MapSheetViewModel` + `MapBottomSheet`: exact money (BigDecimal, no float),
  explicit absence with reason (`no-price`, `anp-no-tariff`), EV-in-ANP switch,
  profile/route/report actions. Hilt defaults to absence (never invents).
- `ChargingMarkersSource` + `MapDiscoveryModule` + `MapSheetModule`: offline
  snapshot (fail-open), explicit absence bindings.
- `MapServerDecodeTest`: staging Go JSON decodes + real localhost HTTP with Go
  bytes (no MockWebServer canned synthesis).

## Verification executed

JVM (`:app:testDebugUnitTest`, `:data:testDebugUnitTest`, green):

- `MapDiscoveryViewModelTest` (8): bounded debounced viewport + stale discard;
  unknown/city-centroid list-only; deterministic clusters + preserved selection;
  cluster expansion without technical IDs; fuel/charging/combined distinct;
  dense 50-marker deterministic; search by city/name bounded + short-query no-wire;
  service filters keep unknown visible.
- `MapSheetViewModelTest` (5): exact product/unit/amount/date; explicit absence
  without fallback; rapid switches discard stale; EV-in-ANP explains + no lookup;
  no-price stays discoverable.
- `MapServerDecodeTest` (2): staging `GET /v1/stations` Go JSON decodes with
  canonical UUIDs/CNPJ leading zeros; same Go bytes over real localhost HTTP
  decode identically (production codec, no invented price).
- `MapScreenViewModelTest`, `MapNavigationTest`, `NavigationTabTest` retain MAP-17.

Phone (2311DRK48G, Android 16):

```sh
./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.app.ui.map.MapScreenInstrumentedTest
# 2 tests PASS — shell, source switch, OSM attribution

./gradlew :maps:engine:connectedDebugAndroidTest \
  -PcomapsSdkDir=/data/dev/artifacts/... \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.maps.engine.MapOverlayListAccessibilityTest
# 2 tests PASS — light/dark TalkBack descriptions, distinct icons
```

Backend (staging `https://teste.abastevo.com.br`, 2026-10-25):

- `GET /v1/stations?limit=5` → 200 with 5 real UUIDs/CNPJs (e.g.
  `0002ead6-…/CENTRO AUTOMOTIVO NEBBIOLO`, all `location_quality:unknown`,
  `coordinates:null`) — decoded by `MapServerDecodeTest`.
- `GET /v1/stations/nearby?lat=-25.42&lon=-49.27&radius_m=10000` → 200 empty;
  Brasilia and charging `places?profile=ocm-br-…` also empty. No REVIEWED
  coordinates and no station prices in staging for pins/prices.

Release APK (with accepted engine + Monocypher + map UI):

- `app-release-unsigned.apk` 78,020,574 bytes / 74.41 MiB, under the 80 MiB
  ceiling (was 74.19 MiB; +0.22 MiB for verifier + UI). Both ARM ABIs contain
  `liborganicmaps.so` + `libmonocypher_ed25519.so`; 16 KB alignment retained
  from MAP-14 (no new native waiver).

## Minimum-gate assessment (honest, no fake PASS)

Proven in app (phone + JVM + real staging bytes):

- Open Mapa → choose ANP/Community → see real staging stations in accessible
  list (UUIDs/names preserved, unknown warnings, no centroid pins).
- Select list → sheet shows explicit absence with quality/location (no invented
  price, no silent fallback, no N+1).
- Open profile (`map/station/<uuid>`) keeps MAP selected; back returns to map.
- Route action fires explicit external `geo:` (documented interim, never
  native guidance).

Blocked by external data (reproduced above, not a code waiver):

- No REVIEWED coordinates in staging (all `unknown` + `null`), so native pins
  cannot appear with real data; dense/cluster/selection proofs use contract
  fixtures + unit tests, not live pins.
- No station-specific ANP/community prices in staging/local test DB, so the
  sheet cannot show a real price/date on-device; exact-money proofs use the
  frozen `poi-assertions-v1.json` (ANP 5890 milli = R$5.89, 2026-10-20) + unit
  tests, not live prices.
- Full search/filter UX (connector/AC/DC/power/access/hours/date scoping) and
  native route preview (`buildCarRoute`) own MAP-22; the interim external route
  is explicit.

Gate: **PARTIAL / INTEGRATION_PENDING**. Do not merge/publish/certify as
complete. Next: seed admitted REVIEWED + price data (or staging with coords/
prices) and rerun the on-device flow; then MAP-21–33 remain out of scope.
