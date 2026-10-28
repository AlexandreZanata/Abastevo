#!/usr/bin/env bash
# Reproducible Abastevo symbol-atlas build (offline, no network, no new tools).
#
# Adds the Abastevo station art (maps/style/symbols/*.svg, from
# docs/assets/map-icons) to the pinned CoMaps default light/dark symbol sets and
# regenerates every density atlas with the pinned skin_generator_tool, exactly
# as tools/unix/generate_symbols.sh does (same sizes, no optipng, which the
# shipped AAR atlases also skip). Without the extra SVGs the output is
# byte-identical to the AAR. Outputs go to app/src/main/assets/symbols/ as
# same-name overrides of the SDK AAR assets; native libraries are untouched.
# The overlay KML selects the art per station via the native CustomImage
# property (maps/engine MapOverlay.kt).
#
# Usage:
#   scripts/maps/build-abastevo-symbols.sh [--comaps-src DIR] [--out DIR]
#
# Defaults: --comaps-src /data/dev/src/comaps-4587b8ad (pinned 4587b8ad, with
# build_skin_generator_tool built), --out app/src/main/assets. The pinned
# source tree is only read.
set -euo pipefail

COMAPS_SRC="${COMAPS_SRC:-/data/dev/src/comaps-4587b8ad}"
OUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --comaps-src) COMAPS_SRC="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    *) echo "build-abastevo-symbols: unknown argument $1" >&2; exit 2 ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${OUT:-$ROOT/app/src/main/assets}"
GENERATOR="$COMAPS_SRC/build_skin_generator_tool/skin_generator_tool"
[ -d "$COMAPS_SRC/data/styles/default/light/symbols" ] || { echo "missing symbols in $COMAPS_SRC" >&2; exit 2; }
[ -x "$GENERATOR" ] || { echo "missing skin_generator_tool: $GENERATOR" >&2; exit 2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/abastevo-symbols.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/styles"
for theme in light dark; do
  cp -r "$COMAPS_SRC/data/styles/default/$theme" "$WORK/styles/$theme"
  for svg in "$ROOT"/maps/style/symbols/*.svg; do
    # Map pins are 26 mdpi units, between bookmark -s (20) and -m (30).
    sed -E '0,/<(ns0:)?svg /s/width="[0-9.]+" height="[0-9.]+"/width="26" height="26"/' \
      "$svg" > "$WORK/styles/$theme/symbols/$(basename "$svg")"
  done
done

export QT_QPA_PLATFORM=offscreen
build_skin() { # theme density size
  local style="$WORK/styles/$1"
  mkdir -p "$WORK/out/$2/$1"
  rm -rf "$style/symbols/png"
  ln -s "$style/$2" "$style/symbols/png"
  "$GENERATOR" --symbolWidth "$3" --symbolHeight "$3" --symbolsDir "$style/symbols" \
    --skinName "$WORK/out/$2/$1/basic" --skinSuffix= >/dev/null
  rm -f "$style/symbols/png"
  mkdir -p "$OUT/symbols/$2/$1"
  cp "$WORK/out/$2/$1/symbols.png" "$WORK/out/$2/$1/symbols.sdf" "$OUT/symbols/$2/$1/"
}
for theme in dark light; do
  build_skin "$theme" mdpi 18
  build_skin "$theme" hdpi 27
  build_skin "$theme" xhdpi 36
  build_skin "$theme" 6plus 43
  build_skin "$theme" xxhdpi 54
  build_skin "$theme" xxxhdpi 64
done
for sdf in "$OUT"/symbols/*/*/symbols.sdf; do
  grep -q 'name="abastevo-station-fuel"' "$sdf" && grep -q 'name="abastevo-station-ev"' "$sdf" \
    || { echo "station art missing in $sdf" >&2; exit 1; }
done
echo "build-abastevo-symbols: wrote $(ls "$OUT"/symbols/*/*/symbols.sdf | wc -l) atlases to $OUT/symbols"
