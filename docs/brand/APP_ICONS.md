# abastevo app icons — P00-T04

The approved A SVG is unchanged; the icon contains no wordmark. White background and the original blue/green gradients are used in the default artwork. [Icon master](../assets/brand/abastevo-app-icon.svg), [1024px export](../assets/brand/abastevo-app-icon-1024.png), [mask preview](../assets/brand/abastevo-app-icon-preview.png) and [provenance](../assets/brand/app-icon-provenance.json) are derived from the approved source.

## Android

Foreground is a native VectorDrawable, with the same paths/clips/gradient stops as the approved vector. White adaptive background; placement inside the 66dp circular safe zone; standard and round launcher references; API33 monochrome silhouette for opt-in themed icons; five density fallbacks. Original geometry/gradients are preserved, not regenerated. No baked outline mask or shadow in shipped artwork. OS/launcher themed mode controls recoloring; the default color icon has white background. This does not lower the existing Android minSdk 26.

## iOS / iPadOS

`iosApp/Assets.xcassets/AppIcon.appiconset` contains opaque RGB PNGs rendered directly from the SVG at exact iPhone/iPad slot sizes (20–180px) plus the 1024px App Store slot. No pre-rounded corners; the OS supplies its mask. The existing iOS host is SPM-only, with no Xcode application target. These launcher assets must belong to the eventual application's main asset catalog, not an SPM library resource bundle. They are prepared, not compiled/shipped proof.

The iOS workstream is deferred by maintainer instruction 2026-10-01 and resumes only on an explicit request. Preserve these assets for that future integration. A future Xcode app target must add `Assets.xcassets` to its resource build phase and set `ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon`; do not claim this unused setting is currently active. There is no Mac/Xcode validation on this host.

## Reproduction and checks

Local validation on 2026-10-01: `./gradlew :app:assembleDebug --no-daemon` PASS (50s); approved master SHA equality, native paths/clips/gradient parity, ten density exports and all eighteen opaque iOS slots PASS. Square/rounded/circular previews inspected; full silhouette fits the adaptive 32.5dp radius. No native iOS build was performed. The user explicitly deferred the iOS workstream on 2026-10-01: preserve these assets and resume only on a new explicit request.

`python3 scripts/brand/export-app-icons.py` requires installed Inkscape/Pillow only as local export tooling, with no runtime dependency addition. SVG remains editable; PNGs are exports for platforms requiring them. Check original master hash, native Android path/gradient parity, safe-zone alpha bounds, iOS exact slot sizes/opaque corners, manifest/resource references and Android assembly. Inspect square/round/circle previews before publication. App behavior, identifiers, license notices and permissions are unchanged.

Sources: [Android adaptive icon guidance](https://developer.android.com/develop/ui/compose/system/icon_design_adaptive), [Apple asset catalog configuration](https://developer.apple.com/documentation/xcode/configuring-your-app-icon).
