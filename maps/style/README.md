# Abastevo clean map style (Uber-like)

Versioned sources for the embedded CoMaps map skin. The pinned SDK only
renders its fixed `BookmarkIcon` set and the drape rules compiled from
MapCSS; custom marker art would require rebuilding the native engine and is
explicitly out of scope (see `docs/assets/map-icons/README.md`).

## Design (light)

- Background `#F7F7F7`, urban landuse `#FFFFFF` (was beige `#F5E8D6`).
- Minor roads stay white with a `#DADADA` casing (`minor_road_border`, casing
  opacity raised to 1 in the shared rules so the border is visible).
- Trunk and motorway fill light gray `#D8D8D8` with `#BDBDBD` casing/tunnels
  instead of orange; primary/secondary keep their official colors.
- Parks `#E3F1E0`, water and wetlands `#D6E4EC`.
- POI icons and POI captions render only at zoom 16+ (rule transform below).
  Geographic names (continents, countries, places, water, parks, capital
  stars) and street names (already dark gray with halo) are untouched.

## Dark variant

Same clean treatment on the dark base: trunk/motorway/primary/secondary use
a neutral gray ramp (`#8C8C8C`/`#757575`/`#666666` with `#4A4A4A` casings),
water `#2F3E45`, parks `#26311F`, minor-road borders `#555555`. POI icons and
POI captions render only at zoom 16+, same as light. The app drives
`MapAppearance` (Light/Dark) from its own theme; see `MapHost(darkMode = …)`.

## Vehicle style

`vehicle/light` and `vehicle/dark` carry the same palettes mapped onto the
vehicle color names (upstream vehicle/light is orange-heavy, vehicle/dark is
brown-heavy). Vehicle rules already hide most POIs; the transform still
applies for consistency. The app does not switch to the vehicle style yet;
the binaries ship ready for guidance mode.

## Sources and provenance

- Base: pinned CoMaps `4587b8ad` (`/data/dev/src/comaps-4587b8ad/data/styles`),
  Apache-2.0. Only these files differ from upstream:
  - `maps/style/colors/abastevo-{default,vehicle}-{light,dark}.mapcss`
    (full replacements of the upstream `colors.mapcss`; same `@` names,
    new hex values only — no classificator or type changes).
  - Rule transform `scripts/maps/abastevo_style_rules.py` (zoom visibility
    to 16 for POI icons/captions; minor-road border opacity `0.1` → `1`).
- Symbols, `drape_colors.mapcss`, `classificator.txt`, `types.txt`,
  `visibility.txt` and `patterns.txt` are reused unmodified from the pinned
  source at build time and are not vendored here.

## Reproducible build

```sh
bash scripts/maps/build-abastevo-style.sh [--comaps-src DIR] [--out DIR] [--keep-work]
```

Defaults: `--comaps-src /data/dev/src/comaps-4587b8ad`,
`--out app/src/main/assets`. Offline; uses the pinned source's existing
`.venv` (protobuf) — no downloads, no new dependencies. The script:

1. Copies the pinned styles into a temp dir (the pinned tree is read-only).
2. Overlays the four Abastevo `colors.mapcss` files.
3. Applies the rule transform (prints per-file raised/dropped counts).
4. Re-runs the transform as a fixpoint check (second pass must be a no-op).
5. Runs the pinned `libkomwm.py` for `default`/`vehicle` × `light`/`dark`.
6. Copies the four `drules_proto_*.bin` files to `--out`.

Verification performed 2026-10-28: regenerating the pristine tree reproduces
the AAR-shipped binaries byte-identically (toolchain trust); the Abastevo
build emits zero non-star POI symbols below zoom 16 and zero references to
the old orange/brown highway colors (see build log). The APK must contain the
new binaries as asset overrides of the SDK AAR (`unzip -l app-debug.apk` plus
`sha256sum` comparison); the native libraries are untouched (MAP-14 SHA pins
still hold).

## Runtime selection

The drape engine picks `default` vs `vehicle` internally (guidance mode) and
`light` vs `dark` via `MapAppearance`. Filenames are unchanged on purpose:
`drules_proto_default_light.bin` and friends ship as same-name asset overrides
in `app/src/main/assets/`, so no native rebuild or style-name plumbing is
needed. Rebuilding the SDK AAR is deliberately avoided: the `.bin` files are
pure data and an asset override has the identical runtime effect without
invalidating the pinned native libraries.
