# MAP-17 — Fourth tab and minimal map shell

Current assessment: [2026-10-25 remediation](MAP-15_20_REMEDIATION_20261025.md) supersedes earlier integration/acceptance claims. Historical evidence below is retained.

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM navigation + ViewModel, real-phone Compose shell)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `3ccb6988` plus MAP-17 changes.
Date: 2026-10-25. No markers, prices or search were added (MAP-18/19/20).

## Behavior frozen

- Bottom bar order is Explorar | Comunidade | Mapa | Perfil with permanent
  labels, equal-width cells and the existing icon system (`Icons.Filled.Map`
  for Mapa). `NavigationTab.fromRoute` keeps `map/station/*` on MAP and
  `station/*` on EXPLORE; premium pages stay on PROFILE.
- `Routes.MAP` (`map`) follows `navigateToTabBase` (singleTop, no child-stack
  restore). `Routes.MAP_STATION_PAGE` (`map/station/...`) opens the existing
  `StationPageScreen` with MAP selected, so back returns to the map base.
  Deep links `anpfuel://map` and `anpfuel://map/station/...` resolve to the
  same destinations.
- The shell contains the real native `MapHost` (Hilt singleton engine),
  an ANP/Comunidade selector at the top, functional filter/search/download
  entries, loading/error/no-package states, safe-area padding, light/dark
  MaterialTheme and a visible `© OpenStreetMap` attribution. No button is
  dead, no fake map/WebView/external shortcut is shipped.
- Public map browsing never requires GPS; permission is requested only on
  explicit recenter (MAP-16). The production APK always ships the accepted
  engine (`:maps:engine` unconditional, fallback to the versioned MAP-14
  candidate when `-PcomapsSdkDir` is absent).

## Implementation

- `AbastevoBottomBar.kt`: fourth tab, updated IA comment.
- `Routes.kt`: `MAP`, `MAP_STATION_PAGE` + `mapStationPage()`.
- `AnpNavGraph.kt`: `MAP`, `map/downloads`, `MAP_STATION_PAGE` destinations.
- `app/ui/map/`: `MapScreen.kt` (selector + host + attribution + entries),
  `MapFiltersDialog.kt`, `MapScreenViewModel.kt` (explicit source identity),
  `MapDownloadsScreen.kt` + `MapDownloadsViewModel.kt` (real active
  generation from `MapPackageStore`).
- `settings.gradle.kts`, `maps/engine/build.gradle.kts`,
  `app/build.gradle.kts`: engine always included; accepted candidate is the
  default when the property is absent.
- Strings `nav_map`, `map_title`, `map_source_*`, `map_search/filters/
  downloads/loading/unavailable/retry/no_package/download/attribution`
  in all eight locales.

## Verification executed

JVM (`:app:testDebugUnitTest`, green):

- `NavigationTabTest` (4 tabs, MAP title/route, MAP vs EXPLORE station
  mapping, null for non-top routes).
- `MapNavigationTest` (order, base/child, deep-link shape, back-to-map,
  no cross-tab hijack).
- `MapScreenViewModelTest` (ANP default, explicit switch, retry clears).

Phone (`:app:connectedDebugAndroidTest`, 2311DRK48G Android 16):

```sh
./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.app.ui.map.MapScreenInstrumentedTest
# Starting 2 tests on 2311DRK48G - 16
# Finished 2 tests on 2311DRK48G - 16
# BUILD SUCCESSFUL
```

- Shell shows ANP/Community selector, Filters/Search/Downloads entries and
  OSM attribution; source switch updates the selector. Real `MapHost` is
  injected via `mapContent` in tests to avoid Hilt; product uses the Hilt
  singleton engine.

Affected `:app:assembleDebug` and `:app:lintDebug` PASS with the engine
included. `git diff --check` clean; no secrets/PII.

## Limits

- No viewport markers, clusters, prices, search or filters yet (MAP-18/19/20).
- Filter dialog currently mirrors the source selector; product/connector/
  power scoping arrives with MAP-20.
- Downloads screen shows the active Room generation; resumable download
  progress UI reuses MAP-15 scheduler and stays pending until MAP-24.
