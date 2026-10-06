# P32-UI-HOME-COMMERCIAL — reference-inspired Home

Status: LOCAL_DONE
Validation: PASS

2026-10-06 user scope: create a polished Home opening inspired by the supplied
mockup, with original branded hero artwork and colored SVG icons matching the
existing filled fuel pictograms. Continue isolated `codex/phase-32-home-minimal`
from `745f747`; preserve other checkouts and all previous navigation adjustments.

B-BR-C01 / BUC-C01: preserve actual municipal ANP averages, observation period,
staleness/offline/error states and source attribution. The mockup's station names,
distances, avatars, consensus and live prices are visual references, not data.
B-BR-C02 / BUC-C02: the contextual hero action selects a station through the
existing station route before capture; no new permission or submission contract.

Presentation: existing abastevo logo, city selector, generated blue/green hero,
economy/community/source-context benefits, clearer ANP reference cards, station
and history shortcuts, preserved vehicles and expert tools. Native text remains
accessible/localized; no text baked into the artwork. SVG masters and equivalent
Android VectorDrawables use filled silhouettes and colored accents; preserve
the existing fuel icons. No dependency, backend/domain or iOS change.

Validation: affected Home/navigation/contrast tests, APK and instrumented-test
builds, lint, selected Home instrumentation on identified Poco, light/dark and
large-font/scroll checks, SVG safety/geometry parity, diff/secret review.
Artwork uses built-in ImageGen; exact prompt/provenance retained with assets.
No remote issue/push/PR/merge/wiki or release certification in this request.

## Assets

- Built-in ImageGen [artwork/provenance](../assets/home-art/README.md) and
  [exact prompt](../assets/home-art/prompt.txt). WebP: 1672 × 941, 58,758 bytes,
  encoded without crop/rescale; loaded locally for offline Home rendering.
- Six [native SVG masters](../assets/home-icons/README.md): piggy bank,
  community, shield, map/stations, history and location. Android path/color
  parity and SVG no-script/no-raster/no-external-reference checks PASS.
- Existing Android logo export copied byte-for-byte; original logo, launcher
  and fuel icons unchanged. No library/font dependency added.
- Eighteen new strings translated in all eight existing locales. ANP is a
  non-translated source label. All copy stays native, including artwork overlays.

## Actual validation

`ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew :app:testDebugUnitTest
--tests '*HomeViewModelTest' --tests '*NavigationTabTest' --tests '*ColorContrastTest'
--tests '*FuelProductTintContrastTest' :app:assembleDebug :app:assembleDebugAndroidTest
:app:lintDebug --console=plain` PASS (2m16s). Home 4, navigation 6, contrast 3,
fuel contrast 2: 15 unit tests, no failures/errors/skips. Hero text/action AA
contrast and explicit app dark-theme fuel colors are covered.

Initial Poco instrumentation: 8/9 PASS, vehicle-card assertion failed because
`performScrollTo` selected the nested horizontal LazyRow rather than its outer
vertical page. Added a carousel test tag and scrolled the actual container
before asserting both vehicle name and total; no assertion removed.
Affected rebuild/APKs/lint PASS (35s). Final targeted class:
`adb -s <identified-phone> shell am instrument -w -r -e class
com.anpfuel.app.ui.home.HomeScreenTest
com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner`
PASS, 9/9 (15.665s), including 2x Compose font scale, dark source label,
contextual contribution -> station selection, city picker, actual fuel route,
empty state, vehicle placeholder and existing registered-vehicle card.

Lint: zero errors, 174 warnings; none suppressed or baselined. Native SVG
contact sheet rendered with existing Inkscape and inspected; six icons have
clear silhouettes and filled accent colors. No renderer dependency installed.
`git diff --check`, localization coverage and scoped secret-surface review PASS.

Final APK installed with `install -r`, data preserved. Cold launch Status ok,
964ms (one sample, not a performance certification). Actual Home opening and
scroll inspected on Poco in light/dark themes: intact logo, readable hero,
colored benefits, source/date/staleness retained and prior navigation styling
preserved. Final screenshots remain only in `/tmp`; observed source data is not
committed. Dark theme restored and Home left open.
APK SHA-256: `dda8b410abad62149bd83190d20028db3b624de07a069b72607c6bd39df6ad10`.

INTEGRATION_PENDING. Existing batch/live-provider/release blockers are untouched.
Rollback: revert this task commit; retain previous Home/navigation commits.
