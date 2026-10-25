# Prepared map station pictograms

Date: 2026-10-24. Status: INTEGRATED_IN_ACCESSIBLE_LIST / NATIVE_MAP_ARTWORK_PENDING.

Original generic artwork generated separately with the built-in ImageGen tool,
following the repository fuel/home icon process. The existing regular gasoline
pictogram was a visual-weight/style reference; no CoMaps/operator logo was used.
The exact prompts and unchanged outputs are retained in sources/.

- `ic_map_station_combustion.svg`: fuel pump with droplet, orange #F57C00.
- `ic_map_station_electric.svg`: charging pedestal with bolt/generic plug, teal #00796B.

These identify **station service types**, not new fuel categories, premium tiers,
connector standards, fuel quality or operational availability. Reusing the fuel
orange is intentional; this is not a new fuel-grade enum or a change to old icons.
The stylized plug is not a technical connector illustration. Always show textual
labels/real connector details and do not rely only on color.

## Reproduction and provenance

Raw transparent PNG -> alpha/ink normalization -> actual traced SVG paths ->
128px Android PNG export. No embedded raster in SVG, no gradients/shadows/fonts/
scripts/external references inside icon masters. A build-time script is provided;
no runtime dependency or application screen was added.

Run from repository root with the already-used local asset tools:

```sh
python3 scripts/assets/trace-map-icons.py --source docs/assets/map-icons/sources/ic_map_station_combustion.png --name ic_map_station_combustion --color '#F57C00' --output docs/assets/map-icons
python3 scripts/assets/trace-map-icons.py --source docs/assets/map-icons/sources/ic_map_station_electric.png --name ic_map_station_electric --color '#00796B' --output docs/assets/map-icons
python3 scripts/assets/fix-map-electric-svg.py --assets docs/assets/map-icons
```

Build-time dependencies match the existing pipeline: Pillow, NumPy, VTracer and
Inkscape. No package was installed for this task. Preserve source image hashes.
Normalization selects alpha >=128, reduces saturated ink to the selected category
and neutral interior details to white, adds even square margins and traces at 2x
nearest sampling without downsampling original source geometry. The alpha, ink
and white IoU checks must each be >=99% against the normalized raster at native
resolution; the pre-correction trace measured scores are 1.0 for both icons. The user then requested filling one unwanted enclosed void in the electric SVG; the corrected SVG is the final authority. That intentional edit is recorded separately, not misreported as fidelity to the unmodified generated mask. This measures raster
mask reproduction, not identical appearance to the original stochastic shading.
The first electric trace failed the white-mask threshold; increased trace sampling
fixed fidelity without lowering the acceptance threshold.

Each provenance JSON records raw/SVG/export hashes, dimensions, path count and
measured scores. Source generation is stochastic; prompts are provenance, not a
promise of exact regeneration. Original generic project assets retain the MIT
notice; do not claim a third-party service mark or icon-set license.

Artifacts: `sources/` untouched PNG/prompts; `transparent/` normalized masters;
SVG masters in this folder; `android/` candidate drawable-nodpi exports.
`preview.png`/`preview.svg` show 24/48/128px on light/dark neutral backgrounds.
Small-size and negative-space silhouettes were visually inspected during planning;
actual map-background, selected-state, 20px and device/theme accessibility review
remain required in MAP-18.

Integration update 2026-10-24: the reviewed Android exports are copied to
`maps/engine/src/main/res/drawable-nodpi/` and used as 24 dp leading icons in
the accessible `MapOverlayList` (fuel, electric, or both side by side). The list
keeps the textual type label and TalkBack description; the icons are decorative
and do not replace the text. The device instrumentation test asserts the list
content description, 48 dp target and both icon compositions; visual appearance
and map-background/20 px review remain MAP-18. The native map overlay still uses the SDK's
`Gas`/`ChargingStation` bookmark symbols; theme-aware native marker backing/
outline/selection, map-background/20 px review and the final MAP-18 artwork
review remain open. Raw large source images stay out of the APK; only the
128 px Android exports are shipped.

## Requested electric SVG correction

The final electric SVG removes exactly one accidental enclosed subpath in the teal
body, left of the lightning bolt. The bolt, display and plug remain intact.
A guarded reproducible repair script verifies the exact subpath signature, fills
only the bounded interior region and proves all pixels outside that region stay
unchanged. It checks full opaque teal at the repaired interior.
The corrected normalized PNG and Android PNG are re-exported from this SVG;
the unchanged generated PNG and pre-repair normalization remain in sources/.
The provenance separates original tracing IoU from the intentional final repair.
