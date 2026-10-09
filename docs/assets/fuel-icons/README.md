# Fuel icon generation and vectorization

The premium gasoline icon was generated with the built-in image generation tool
on 2026-10-09, using the existing additived-gasoline PNG as a style reference.
This is original generic pump artwork, not a Petrobras/Podium logo. See the
[premium category contract](../../mobile/premium-fuel-category.md).

## Repeatable workflow

1. Inventory **actual PNG/SVG inks**, not only Compose text tints. Current fuel
   art uses orange (common gasoline/LPG), gold (additive gasoline/CNG), green
   (ethanol), blue (S500), violet (S10). Do not copy historical duplicate colors
   into a new category. Ruby/magenta **#C2185B** is reserved for true premium
   gasoline; its crown signals quality. Every future category needs one unique,
   meaningful category ink. White interior details and transparency are neutral.
2. View representative existing assets. Preserve front pump, rounded display,
   droplet, right hose/nozzle, bottom plinth, visual weight and proportions.
   Generate a transparent PNG first. Ask for flat solid category ink, white
   interior details, no text/logos/gradients/shadows. Keep the exact prompts in
   `sources/`; retain the selected raw output unchanged. Inspect alpha and shape.
3. The first premium draft was violet. Inspection of all actual icons revealed
   violet already belongs to S10, so the final image-generation edit changed
   only the ink to ruby/magenta. Only the selected final raw PNG is shipped in
   this repository; generation history is documented, not passed off as native
   vector design. The generator may still produce slight shading; normalize it
   deterministically before tracing.
4. Run from the repository root:

   ```sh
   python3 scripts/assets/trace-fuel-icon.py --source docs/assets/fuel-icons/sources/GASOLINA_PREMIUM_GENERATED.png --name gasoline_premium_grade --color '#C2185B' --size 128
   ```

   The script checks dominant existing hues (20-degree minimum separation),
   keeps original source pixels while centering on a transparent square,
   removes generated shading to one category ink plus white, traces both masks
   with VTracer spline curves, and exports the SVG through Inkscape to Android.
   Input masks use alpha >=128; saturated pixels are category ink, neutral
   interior pixels white. This is real vectorization, never a PNG embedded in
   an SVG wrapper. Inspect small isolated details; do not tune thresholds merely
   to hide a poor generation. The 20-degree guard is an asset-tool policy, not a
   substitute for visual/color-vision accessibility review.
5. Deliver all four artifacts: raw generated PNG in `sources/`, normalized PNG
   in `transparent/`, SVG master here, Android PNG under `drawable-nodpi/`.
   Gasoline icons export to **128 x 128**; Compose retains its existing dp sizes.
   Ethanol is a historical 256px exception, not the new gasoline export target.
   Do not install large traced path strings directly into Android VectorDrawable:
   the existing set uses local PNG exports and SVG documentation masters.
6. Inspect PNG and SVG render on light/dark backgrounds at 20/24/48/128px; check
   crown/display/nozzle holes and padding. The provenance JSON records source
   SHA256, path count, dimensions, selected ink and silhouette intersection over
   union against the normalized master at native resolution. Require >=99%.
   This metric measures silhouette, not perceptual identity of every internal
   pixel. Compare category/white masks too before claiming complete fidelity.
7. Map the new enum to its own resource and localized label; retain textual
   category labels for accessibility. Check light/dark text-tint contrast and
   build/lint the actual APK. Never reuse the legacy additive icon/enum for true
   premium. Preserve existing assets and hashes.

## Tools and provenance

Pillow, NumPy, VTracer and Inkscape are build-time asset tools already available
on this workstation, not app/runtime dependencies. Re-run tool/license/security
review before introducing new dependencies elsewhere. VTracer's spline trace
settings are recorded directly in the script; raw source and prompts permit
reproduction without calling the image generator again. Model generation itself
is nondeterministic; do not claim a seed or exact regeneration guarantee.

Final measured metadata: [provenance](ic_fuel_gasoline_premium_grade.provenance.json).
The normalized master is the final one-ink PNG; the untouched generated PNG
preserves model output for future investigation.
