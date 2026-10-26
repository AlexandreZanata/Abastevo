# Map controls and filter presentation refinement

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Parent: `6a689e585cacd0948b6f14330de5282fe31a5ef6` (`dev`, PR #154)
Date: 2026-10-26

## B-BR / BUC before implementation

- B-BR-MAP-UI-06: the initial map overlay is compact. Search and current city
  stay visible; a labelled expand action exposes source, survey period and
  offline availability details without resizing or recreating the map.
  Current source and missing-package status remain identifiable when collapsed.
- B-BR-MAP-UI-07: filters use a scrollable Material sheet, grouped sections,
  wrapping chips, existing fuel pictograms with text, and pinned Apply/Cancel.
  Source/service/product/charging edits remain drafts until Apply; Cancel drops
  those drafts. Existing source, unknown-metadata and filtering rules stay intact.
- B-BR-MAP-UI-08: a city search field opens the existing LocationPickerScreen
  inside the map filter flow. This reuses its full catalog, state and municipality
  search, accent normalization, loading/empty/error states and selection storage.
  Selecting a location keeps the existing immediate preference-save semantics;
  cancelling the filter does not undo a separately confirmed location change.
  Returning from location search retains the other filter drafts.
- BUC-MAP-UI-02: open Map -> search or expand details -> open Filters -> select
  fuel by icon and name -> search state/city with the standard location picker ->
  return to the filter sheet -> Apply -> browse the unchanged native map host.

Scope: Android UI only; no new dependency, backend, provider, permission,
GPS activation, price calculation, deployment or merge. Original occupied
checkout and unrelated worktrees are preserved. Delivery is a new commit on
the existing PR #154 head; no merge requested.

## Implementation and verification

- `MapControlsCard`: compact city/source/product summary, station search with
  keyboard Search action, and explicit expand/collapse. Source/period/route and
  detailed offline states stay available; the native host keeps its full bounds.
  The bottom action panel and first-frame skeleton follow the compact geometry.
- `MapFiltersDialog`: themed Material bottom sheet, pinned footer, grouped source,
  service, fuel and electric options. FlowRows wrap every choice; existing
  `FuelProductIcon` assets distinguish all eight fuels alongside their labels.
  Applying invokes the selected source and extended filters once; cancellation
  invokes neither. Conditional service sections retain their values in drafts.
- City field uses the actual `LocationPickerScreen` and existing view model with
  a map-specific key. The full catalog/state/municipality search and its error,
  loading, empty and permission handling stay shared. The filter sheet remains
  composed underneath, preserving its drafts. Existing preferences are refreshed
  on selection; no ten-city fallback list is used by the production filter UI.
- All three new presentation strings cover all eight supported resource locales.
  No dependency or Gradle/manifest/domain/backend/engine implementation changed.

Commands executed from the isolated worktree:

```sh
./gradlew :app:testDebugUnitTest --tests '*Map*' --tests '*EvTrip*' \
  --tests '*LocationPicker*' :maps:engine:testDebugUnitTest \
  :app:assembleDebug :app:assembleDebugAndroidTest --offline --console=plain
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug \
  --offline --no-daemon --max-workers=2 --no-parallel --console=plain
./gradlew :app:lintDebug --offline --no-daemon --max-workers=1 \
  --no-parallel --console=plain
adb -s VKCQGAUCGYAMR4W4 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s VKCQGAUCGYAMR4W4 install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
adb -s VKCQGAUCGYAMR4W4 shell am instrument -w \
  -e class com.anpfuel.app.ui.map.MapScreenInstrumentedTest \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

- App JVM: **92 tests, zero failures/errors/skips**; engine: **37 tests, zero
  failures/errors/skips** (engine unchanged, cache reused). Included six existing
  LocationPickerFilter cases preserve normalization and state/city matching.
- Debug app and instrumentation APKs: **PASS**, final installation succeeded with
  `-r`; no uninstall/data clear, new package or signing-key change.
- Native phone: **9/9 PASS in 21.248 s**. Default light fixtures plus explicit
  dark/200% font fixture cover the compact/expanded controls, source drafts,
  cancellation, icon/selectability of every fuel, all electric controls and
  pinned footer. Existing cluster/charging/source regressions remain intact.
  [Native runner output](map-ui-refinement-20261026/native-tests.txt).
- Initial native run was 8/9: the inherited test tried to scroll the former
  horizontal parent, which is now the vertical scroll node itself. Removed that
  obsolete parent-scroll call; direct scroll-to-option, displayed/selected and
  applied-value assertions all remain and pass. No test skipped or weakened.
- Android lint: **PASS, zero errors**, final `BUILD SUCCESSFUL in 7m 31s`.
  Earlier foreground processes exited with SIGTERM before a report and were not
  counted as success. The final single-worker run completed all main/test checks;
  no baseline, suppression or detector/configuration change was introduced.
- Final staged diff/secret-surface review: **PASS**. All evidence links resolve,
  source fingerprints match, and all eight locales contain the new strings.

## Manual phone evidence

Real app flow: Map -> Filters -> draft Community + Diesel S10 -> city search ->
search state `MT` -> Mato Grosso -> search `Sorriso` -> select the already
preferred SORRISO -> return. Community and Diesel S10 remain selected in the
sheet, with Apply/Cancel still available. No different preferred city was saved.
The native host remains behind the flow; browsing does not start GPS.

- [Compact native map](map-ui-refinement-20261026/map-compact.png).
- [Grouped filters and all fuel icons](map-ui-refinement-20261026/filters-dark.png).
- [Standard state search](map-ui-refinement-20261026/location-state-search.png).
- [Standard municipality search](map-ui-refinement-20261026/location-city-search.png).
- [Drafts after returning from city search](map-ui-refinement-20261026/filters-city-return.png).

Final debug APK SHA-256:
`278e1289477de972aad620938b6bbf7a0d0cee14c2e80e8fbb0a708a1c80ea13`.

Tested presentation fingerprints (SHA-256):

- MapControlsCard: `c322c3747299f49a6d9e61eb5ec35209dc9c1cd342eede43f9118249d72db939`.
- MapFiltersDialog: `e95e5c6c393a675b351cccc2f989c293918a1a421f3d041e1c38be724f8e264a`.
- MapScreen: `1a620f807eaae09f0c743d2f59eee0708833ce7ca650d759ba1f2c41c683cf0c`.
- Native test: `e97d3e4ae5002dd4d5d766df8404190d52e472ac8a82d04011d51e778b64bb49`.

## Delivery and limits

PR #154 remains the delivery target (`dev` -> `main`); no merge requested.
Origin identity and push permission were verified. Main protection was read live:
strict required `Quick verification`, enforced for admins. A normal fast-forward
push must preserve any concurrent remote work; CI for the new head is pending.

The original checkout and occupied local `dev` worktree stay untouched. The
isolated worktree starts from the verified remote `dev` head. Existing staging
empty-data/fixture-location/offline-package obligations remain unchanged; no
source/data completeness, staging deployment, release or production claim.
Rollback is a new commit reverting this presentation change; no schema/data
recovery is needed. Wiki publication waits for a separately authorized merge.

## Follow-up: left-aligned city and shared fuel selector

Parent: `cb111741`. Status: LOCAL_DONE / INTEGRATION_PENDING. Validation: PASS.

- B-BR-MAP-UI-09: city icon/name align to the start of the header container.
  Long names use one visual line with end ellipsis, while keeping the full name
  available to accessibility. Source and expansion controls keep their space.
- B-BR-MAP-UI-10: fuel uses the existing full-width
  `FuelProductSelectorButton` and `FuelProductSelectorDialog`, as in VehicleForm.
  A single current value replaces adjacent fuel chips. The shared modal retains
  all eight named/icons options; selecting updates the filter draft only.
- BUC: open filters -> press the current fuel field -> select a fuel in the
  standard modal -> return to the field -> Apply, or Cancel to discard the draft.

Scope remains presentation only. Reuse the clean isolated worktree and existing
PR #154; original occupied checkout stays untouched. Verify city alignment and
actual ellipsis, all fuel choices through the shared modal, 200% font, affected
native regressions, APK compilation, lint, diff and secret surface before push.

### Follow-up results

- Reused the existing VehicleForm fuel button and modal directly. The only shared
  component API addition is an optional Modifier (unchanged default behavior),
  plus stable icon semantics tags for the existing eight-option regression.
- Debug app/test APKs and Android lint PASS (`BUILD SUCCESSFUL in 9m 58s`),
  zero errors; the report retains 284 warnings / two informational findings.
- `MapScreenInstrumentedTest`: **10/10 PASS in 83.052 s**, isolated Android 11
  API30 emulator with existing ARM translation and KVM, no source ABI/runtime
  configuration changes. Every fuel is selected through the standard modal,
  reflected in the field, and withheld from the apply callback until Apply.
  Outer Cancel still discards the changed fuel/source draft. The 200% dark-font
  case repeats all eight modal selections and electric/footer reachability.
- City regression measures the short name's start offset within 48 dp of the
  header edge and verifies actual `TextLayoutResult.isLineEllipsized(0)` for a
  long name at 320 dp width, one line, full accessible text and visible controls.
- The connected API35 16 KB emulator first returned DeadObjectException before
  starting instrumentation, then an ART native crash; other system apps also
  reported ART failures. These attempts are not accepted as passing evidence.
  The existing emulator was preserved. An isolated temporary API30 image already
  installed in the SDK was used for the completed suite; no dependency added.
  The temporary emulator was stopped and its task-owned AVD/home removed afterward.
- The physical phone was disconnected for this revision. No current phone or
  16 KB runtime acceptance is claimed; prior phone evidence above belongs to its
  original checkpoint. No test was skipped; the failed runtime attempts are explicitly retained here.
- No domain/data/backend/engine implementation changed, so the previous 92 app /
  37 engine JVM baseline was not repeated for these presentation-only edits.
- Diff and secret-surface review PASS; origin/dev parent was verified before work.
  Deliver as a new commit to PR #154, preserving prior commits, without merge.

```sh
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug \
  --offline --max-workers=1 --no-parallel --console=plain
adb -s emulator-5556 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s emulator-5556 install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
adb -s emulator-5556 shell am instrument -w \
  -e class com.anpfuel.app.ui.map.MapScreenInstrumentedTest \
  com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

[Follow-up native runner output](map-ui-refinement-20261026/native-tests-followup.txt).
The earlier screenshots document the first checkpoint; the shared selector modal
supersedes their adjacent fuel chips in this follow-up.

Follow-up APK SHA-256:
`4db036586e9be226be22f4f2a7c3c0795dd9a2400746bd25d21b902d6a5e32ac`.

Follow-up tested source SHA-256:

- MapControlsCard: `0032240794ca09c9dbdf47e6ef4cb4d2968c7427d88f1f5637f1ea3d073c78aa`.
- MapFiltersDialog: `777223358188723ec45adcdc56e8c80e1d92f5d34dc3b78d9e09e875c93818a5`.
- FuelProductSelector: `af6e6e91400e55d96efc734cde204f2d61ab59b98bb14bf6dd50d8e7eff1588e`.
- MapScreenInstrumentedTest: `75bbd965299428a4698932ad44f25ce273849ff00d0350a277854f483db41fff`.
