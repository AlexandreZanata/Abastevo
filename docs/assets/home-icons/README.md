# Home pictograms

2026-10-06 single-ink replacement of the original Home pictograms. Each new
icon was generated separately with the built-in ImageGen tool, using the
approved ethanol source as a style reference. Exact prompts and generated raw
PNGs are retained in [sources](sources/). The corrected pig replaces the first
candidate with its accidental central alpha/shading artifact.

Pipeline: generated PNG → single-ink silhouette quantization → VTracer 0.6.15
spline SVG → sharp 256 × 256 Android PNG. Reproduce with the existing
local Pillow/NumPy/OpenCV/VTracer tools:

```sh
python3 scripts/assets/trace-home-icons.py
```

No runtime dependency or new package was added. The tool retains the source
silhouette at its native raster resolution (no
pre-trace reduction). Double sampling provides subpixel spline precision;
each SVG has a source-derived square viewport and even margins, native size
64 dp. Edges receive a 2.4-pixel sampling filter, and curves use eight decimal
places. All six full-resolution silhouette comparisons exceed 99.5% IoU.
[Preview](preview.png). All filled
paths of an icon have exactly one category color; details are transparent
negative space. No gradients, shadows, highlights, scripts, fonts, embedded
images or external SVG references. PNG exports are rendered directly from the SVG masters. The app keeps
`Color.Unspecified`.

- [Savings / piggy bank](ic_home_savings.svg), green `#209437`.
- [Community](ic_home_community.svg), blue `#1478DD`.
- [Trust / shield](ic_home_trust.svg), orange `#F57C00`.
- [Stations / map](ic_home_stations.svg), blue `#1478DD`.
- [History / chart](ic_home_history.svg), green `#209437`.
- [Location](ic_home_location.svg), blue `#1478DD`; retained as an asset,
  not reintroduced beside the city name.

[Transparent single-ink PNG exports](transparent/) are also saved. The app uses
local 256 × 256 PNGs in `app/src/main/res/drawable-nodpi/ic_home_*.png`, matching
the existing fuel asset pipeline. SVG masters scale without resolution loss; raw
ImageGen sources are not shipped in the APK. Icons remain decorative beside
accessible native labels. The shield represents source context, not certified
price quality. Original logo and other fuel icons are unchanged.

The separately generated ethanol-white preview and prompt are retained in
sources for provenance. The selected ethanol SVG preserves the approved SVG's
exact green geometry/fill and adds an opaque white leaf using the same outline
coordinates. This honors the user's exact-shape constraint; a generative edit
cannot guarantee identical geometry. Existing fuel PNG provenance is retained.
