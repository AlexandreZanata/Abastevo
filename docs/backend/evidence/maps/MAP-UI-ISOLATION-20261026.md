# Map page isolation, stable geometry and professional loading state

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS for the local UI/phone checks below
Branch: `codex/map-ui`, parent `fe6ca851` (maintained dev)
Date: 2026-10-26

## B-BR / BUC

- **B-BR-MAP-UI-01 (persistent map host):** the native `MapHost` lives in the app
  shell, not inside the `NavHost` destination. Once the Map tab is opened it stays
  composed; switching to Explore/Community/Profile only hides it and stops camera
  polling. Re-entering Map does not reinitialize the engine or recreate the map
  surface.
- **B-BR-MAP-UI-02 (stable layout):** the map fills the available content area from
  first composition. Search, filters, city, survey week, download warning and the
  action/list panel are overlays on top of the map, so their arrival never changes
  the map bounds. The map no longer starts at a larger height and shrinks when the
  fields load.
- **B-BR-MAP-UI-03 (skeleton):** first map entry shows a `MapScreenSkeleton` built
  from the shared `SkeletonGroup`/`SkeletonLine`/`SkeletonCard`/`SkeletonButton`
  primitives, matching the final top/bottom overlay geometry. It remains until the
  native engine reports a rendered frame, not merely initialization. No spinner
  replaces the map.
- **B-BR-MAP-UI-04 (tab transition):** top-level tab switches use no crossfade, so
  the previous page cannot ghost over the persistent map while the next tab enters.
  Non-tab navigation keeps the existing fade.
- **B-BR-MAP-UI-05 (safety):** browsing still does not start GPS/TTS/recording; the
  map visibility flag only controls rendering and viewport polling. The recenter
  control is offset above the bottom overlay. No new dependency, permission,
  tenant, upload or private-data path is added.
- **BUC:** user opens Map -> sees a map-shaped skeleton with the final control
  geometry -> map renders in place -> switches to another tab and returns -> map is
  already visible, with no engine restart or resizing.

## Implementation

- `maps/engine/MapHost.kt`: `onReady` now waits for `engine.isRenderingReady()`;
  `active` hides the `AndroidView` and stops the 600 ms viewport poll when the map
  is not visible; `recenterBottomInset` keeps the recenter control above the new
  bottom panel.
- `MapViewModel.kt`: exposes rendering readiness.
- `AnpNavGraph.kt`: owns the persistent MapHost and Activity-scoped
  `MapDiscoveryViewModel`; the Map destination receives both. Other pages get an
  opaque background over the hidden map. Tab-to-tab transitions are `None`.
- `MapScreen.kt`: full-size map with translucent rounded top/bottom control
  surfaces; `FlowRow` chips; inline loading/error/download states; 48 dp targets;
  `mapReady` skeleton gate; no map replacement spinner.
- `MapScreenSkeleton.kt`: shared-skeleton map placeholder with top/bottom control
  silhouettes.
- `MapScreenInstrumentedTest.kt`: updated the functional-entry assertion to the
  stable `map-open-downloads` tag because “Download” now appears both in the
  warning and the action bar.

## Verification

- `:app:compileDebugKotlin` PASS.
- `:app:testDebugUnitTest --tests '*Map*' --tests '*EvTrip*'` +
  `:maps:engine:testDebugUnitTest` PASS.
- `:app:lintDebug` BUILD SUCCESSFUL.
- `MapScreenInstrumentedTest` 5/5 on the authorized phone
  `VKCQGAUCGYAMR4W4` (Android 16, arm64, real app package).
- Manual phone flow: first Map entry shows the skeleton; ready map keeps the same
  bounds; Explore -> Map at ~0.18 s shows the already-rendered map, no skeleton and
  no previous-page ghost. Screenshots:
  [skeleton](map-ui-isolation-20261026/map-skeleton.png),
  [ready](map-ui-isolation-20261026/map-ready.png),
  [instant return](map-ui-isolation-20261026/map-tab-return.png).

## Limits

- The staging `nearby` is still empty and the selected city is Sorriso with
  `Location unknown`; that data issue is independent of this UI change and the VPS
  data update remains pending. The local map is rendered from the admitted MWM
  fixture; the “No offline package” warning correctly remains until the product
  package is downloaded/configured.
- This branch is installed on the phone but is not merged, pushed, released or
  certified. No production/staging change was made. A later delivery can carry the
  branch through the protected dev → main flow.
