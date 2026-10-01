# abastevo logo assets

The maintainer-supplied original is `abastevo-logo-source.png`. Keep it unchanged. The vector master is `abastevo-logo.svg`: transparent background, native spline paths, clipped folds and linear gradients. It contains no embedded raster, font, script or external resource. It scales without raster pixelation.

PNG does not retain the original vector construction. These outlines and gradients are reconstructed; this is not a mathematically lossless PNG-to-vector conversion. Small optical differences remain, and faint raster specks are intentionally removed. Do not describe this as the original editable artwork recovered exactly.

## Future app imports

- **Master / documentation:** use [abastevo-logo.svg](abastevo-logo.svg). Edit paths/gradient stops in a vector editor. The 1254 × 1254 viewBox preserves the supplied square composition and transparent margins.
- **Android:** copy [abastevo_logo_android.xml](abastevo_logo_android.xml) to the selected Android module's `res/drawable/abastevo_logo.xml` when branding is implemented. It uses native VectorDrawable paths, clips and complex-color gradients; the isolated resource fixture compiled and linked with build-tools 35.0.0 / android-35 / minSdk 26. This validation does not certify a launcher/adaptive icon or device rendering. Preserve the existing package identifiers.
- **iOS:** import [abastevo-logo-ios.pdf](abastevo-logo-ios.pdf) into a future image asset, select Single Scale and Preserve Vector Data in Xcode, then verify the actual app/device rendering. The one-page PDF contains vector geometry/gradients and zero embedded raster images. It is a reusable image asset, not an AppIcon set.
- **Preview only:** [abastevo-logo-preview.png](abastevo-logo-preview.png) is a 256 px raster render for quick visual inspection; use a vector file as the master.

No runtime resources or app icons have been replaced. Platform optical tests and final launcher/App Store packaging belong to the app branding task. The maintainer approved README V2 publication on 2026-10-01.

## Local evidence (P00-T02, 2026-10-01)

Inkscape 1.2.2 rendered the SVG at 32, 64, 256, 1254 and 4096 px. Visual inspection covered the original comparison, internal folds, transparent gaps and light/dark backgrounds. At 1254 px and alpha > 128, silhouette intersection-over-union is 0.994204; this measures outline overlap, not color fidelity. SVG safety, exact SVG/Android visible-path and gradient-stop parity, Android resource compile/link, PDF raster inventory and byte-for-byte original preservation passed. No iOS/Android device runtime test was run for these unused assets.

[vector-provenance.json](vector-provenance.json) records method, tool versions, checks, sizes and SHA-256 hashes. README composition provenance and prompt remain separate. Refer to [identity documentation](../../brand/IDENTITY.md) and the repository's rights notices before redistribution.


## Reference lettering / README V2 (P00-T03)

[abastevo-wordmark.svg](abastevo-wordmark.svg) traces only the supplied ABASTEVO text into native paths with transparent background and navy fill; the reference logo is excluded. [abastevo-readme-banner-v2.svg](abastevo-readme-banner-v2.svg) combines that text with the unchanged original-logo SVG in the existing white horizontal banner. [PNG preview](abastevo-readme-banner-v2.png) is a render, not the editable master. [Wordmark provenance](wordmark-provenance.json) records source identity and validation. Uppercase is specific to this artwork; the project name in prose remains lowercase. V2 is approved for publication; V1 remains preserved as historical composition evidence.
