# P32-UI-ICON-MONOCHROME

Status: LOCAL_DONE
Validation: PASS

2026-10-06 user scope, from `5cad0f6` on isolated
`codex/phase-32-home-minimal`: generate a minimalist raster image for each new
Home pictogram, then trace to high-resolution SVG and export Android PNGs, matching the
existing fuel pipeline. Savings, community,
trust, stations, history and location each use one solid category color;
no gradients, highlights, shadows or extra colors. Preserve Home layout and
header branding. Reuse existing tools and record prompts and source provenance.

B-BR-I01 / BUC-I01: ethanol retains its approved droplet/leaf geometry and green;
only the internal leaf becomes opaque white in both themes, matching the white
details of existing fuel icons. Preserve every other fuel asset byte-for-byte.
B-BR-I02 / BUC-I02: generated native icon paths remain local/offline, decorative
beside accessible native labels, with no external SVG references or scripts.
No source/navigation/domain/backend behavior or dependency change.

Validation planned: raster/vector inspection, path/fill/safety and other-fuel
hash checks; affected unit checks, both APKs and lint; targeted Home/fuel device
checks including ethanol white pixels on dark background; Poco update preserving
data and both-theme visual review. Atomic commit and authorized branch backup
push; no PR, merge or release certification inferred.


## Actual artwork and vector quality

Built-in ImageGen created six separate pictograms and an ethanol-white preview;
exact prompts and raw selected PNGs are in `docs/assets/home-icons/sources`.
The first pig had a central transparency/shading patch; a referenced edit still
retained it. Replaced both with a fresh opaque-white-background generation;
only that clean selected image is committed. Its final transparent PNG and SVG
have a uniformly opaque `#209437` body, confirmed visually and by three interior
RGBA samples. Other selected images were not regenerated after the correction.

Existing local VTracer 0.6.15, Pillow, NumPy, OpenCV and Inkscape perform
single-ink quantization and tracing. No dependency was installed or added to
Android. Preserve native source silhouette resolution (958–1215 px); double
sampling gives subpixel spline precision, eight decimal places. Small 2.4-pixel
edge filtering removes raster alias noise. Earlier curve configurations failed
the 99.5% fidelity target; refined sampling/curve parameters rather than weakening
that target. Final full-resolution alpha-mask IoU against prepared PNGs:
savings 99.82485%, community 99.79916%, trust 99.84820%, stations 99.88937%,
history 99.96317%, location 99.76628%. All exceed 99.5%; enlarged native renders
and the committed `preview.png` were inspected. SVGs contain only local paths,
one fill per pictogram, no gradients, scripts, external references or bitmaps.

Ethanol's original SVG paths/fills/transforms are preserved byte-for-byte as
attributes; insert one opaque white leaf path using those same outline points.
The generated ethanol preview is retained as provenance, not used to replace
the exact approved geometry. The green silhouette and all 18 other fuel SVG/PNG
assets were checked against `5cad0f6` and are unchanged. The Android ethanol
PNG is regenerated from that corrected master at 256 × 256.

## Runtime correction and checks

Initial native VectorDrawables were rejected at runtime: instrumentation crashed
with `Unknown command for: R`; aapt logged `STRING_TOO_LARGE` for long vector
path strings even though Gradle reported success. Those Android XML exports
were removed. Full-resolution SVG masters stay in docs; Android uses local
256 × 256 PNGs rendered directly from the masters, as with the existing fuels.
Six new runtime PNGs total 41,128 bytes. No incompatible vector path is shipped.

Final scoped command:
`ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew :app:testDebugUnitTest
--tests '*HomeViewModelTest' --tests '*NavigationTabTest' --tests '*ColorContrastTest'
--tests '*FuelProductTintContrastTest' :app:assembleDebug :app:assembleDebugAndroidTest
:app:lintDebug --console=plain` PASS (57s). 15/15 unit tests; both APKs and lint
PASS, zero errors / 175 warnings (existing unused location export retained).
No suppressions, baselines or weakened test assertions.

Poco instrumentation for `HomeScreenTest,FuelProductIconScreenshotTest` PASS,
12/12 (18.17s), including 2x-font equal columns, preserved navigation/vehicles,
both fuel layouts and opaque white ethanol-leaf pixels in light/dark surfaces.
Installed with `install -r` on identified Poco only; app data preserved.
Actual dark/light Home inspected; no central pig patch, sharp single-ink
benefits, white ethanol leaf, unchanged header/layout. Screenshots remain in
`/tmp`, outside Git. Original dark theme restored. Cold launch Status ok, 905ms
(one sample; not a performance certification). Diff and secret-surface review
PASS. APK SHA-256:
`4c7861a0d24777e5a155e0c311ca9b31784bc9a0afbbcb97e886fa83c99722dc`.

INTEGRATION_PENDING. Existing runtime/provider/release blockers are unchanged.
Rollback: revert this task commit; retain previous Home and navigation commits.
