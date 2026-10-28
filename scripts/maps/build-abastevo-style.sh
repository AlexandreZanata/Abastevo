#!/usr/bin/env bash
# Reproducible Abastevo map-style build (offline, no network, no new tools).
#
# Assembles the pinned CoMaps style sources, overlays the versioned Abastevo
# colors (maps/style/colors/), applies the versioned POI/casing rule transform
# (scripts/maps/abastevo_style_rules.py) and regenerates the drape rule
# binaries with the pinned kothic generator. Outputs go to app/src/main/assets/
# as same-name overrides of the SDK AAR assets (no AAR rebuild needed: the
# .bin files are pure data loaded at runtime; native libraries are untouched).
#
# Usage:
#   scripts/maps/build-abastevo-style.sh [--comaps-src DIR] [--out DIR] [--keep-work]
#
# Defaults: --comaps-src /data/dev/src/comaps-4587b8ad (pinned 4587b8ad),
# --out app/src/main/assets. The pinned source tree is only read.
set -euo pipefail

COMAPS_SRC="${COMAPS_SRC:-/data/dev/src/comaps-4587b8ad}"
OUT=""
KEEP_WORK=0
while [ $# -gt 0 ]; do
  case "$1" in
    --comaps-src) COMAPS_SRC="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --keep-work) KEEP_WORK=1; shift ;;
    *) echo "build-abastevo-style: unknown argument $1" >&2; exit 2 ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${OUT:-$ROOT/app/src/main/assets}"
VENV="$COMAPS_SRC/.venv/bin/python"
LIBKOMWM="$COMAPS_SRC/tools/kothic/src/libkomwm.py"

[ -d "$COMAPS_SRC/data/styles/default" ] || { echo "missing styles in $COMAPS_SRC" >&2; exit 2; }
[ -x "$VENV" ] || { echo "missing pinned venv python: $VENV" >&2; exit 2; }
[ -f "$LIBKOMWM" ] || { echo "missing libkomwm: $LIBKOMWM" >&2; exit 2; }
command -v patch >/dev/null || { echo "missing: patch" >&2; exit 2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/abastevo-style.XXXXXX")"
trap '[ "$KEEP_WORK" = 1 ] || rm -rf "$WORK"' EXIT
echo "work dir: $WORK"

cp -r "$COMAPS_SRC/data/styles" "$WORK/styles"
for f in colors.txt types.txt visibility.txt patterns.txt mapcss-mapping.csv mapcss-dynamic.txt; do
  cp "$COMAPS_SRC/data/$f" "$WORK/$f"
done

declare -A COLOR_SRC=(
  [default/light]=abastevo-default-light.mapcss
  [default/dark]=abastevo-default-dark.mapcss
  [vehicle/light]=abastevo-vehicle-light.mapcss
  [vehicle/dark]=abastevo-vehicle-dark.mapcss
)
for style_mode in "${!COLOR_SRC[@]}"; do
  src="$ROOT/maps/style/colors/${COLOR_SRC[$style_mode]}"
  [ -f "$src" ] || { echo "missing colors: $src" >&2; exit 2; }
  cp "$src" "$WORK/styles/$style_mode/colors.mapcss"
done

python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/default/include/Icons.mapcss"
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/default/include/Basemap_label.mapcss" --basemap-label
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/default/include/Subways.mapcss"
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/default/include/Roads.mapcss"
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/vehicle/include/Icons.mapcss"
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/vehicle/include/Basemap_label.mapcss" --basemap-label
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/vehicle/include/Subways.mapcss"
python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$WORK/styles/vehicle/include/Roads.mapcss"

# Fixpoint check: a second transform pass must be a no-op (proves the rewrite
# is stable and the parser round-trips every rule byte-identically). Only the
# transformed files are checked; Roads_label keeps street names at all zooms.
for f in \
  "$WORK/styles/default/include/Icons.mapcss" \
  "$WORK/styles/default/include/Basemap_label.mapcss" \
  "$WORK/styles/default/include/Subways.mapcss" \
  "$WORK/styles/default/include/Roads.mapcss" \
  "$WORK/styles/vehicle/include/Icons.mapcss" \
  "$WORK/styles/vehicle/include/Basemap_label.mapcss" \
  "$WORK/styles/vehicle/include/Subways.mapcss" \
  "$WORK/styles/vehicle/include/Roads.mapcss" ; do
  before="$(sha256sum "$f" | cut -d' ' -f1)"
  args=()
  case "$f" in *Basemap_label.mapcss) args=(--basemap-label) ;; esac
  out="$(python3 "$ROOT/scripts/maps/abastevo_style_rules.py" "$f" "${args[@]}")"
  after="$(sha256sum "$f" | cut -d' ' -f1)"
  if [ "$before" != "$after" ]; then
    echo "fixpoint FAILED for $f (second pass changed it): $out" >&2
    exit 2
  fi
done
echo "rule transform fixpoint verified"

run_style() {
  local style="$1" mode="$2" suffix="$3"
  echo "generating drules for $style/$mode"
  "$VENV" "$LIBKOMWM" --txt \
    -s "$WORK/styles/$style/$mode/style.mapcss" \
    -o "$WORK/drules_proto_$suffix" \
    -p "$WORK/styles/$style/include/"
}

run_style default light default_light
run_style default dark default_dark
run_style vehicle light vehicle_light
run_style vehicle dark vehicle_dark

mkdir -p "$OUT"
for suffix in default_light default_dark vehicle_light vehicle_dark; do
  bin="$WORK/drules_proto_$suffix.bin"
  [ -f "$bin" ] || { echo "missing output: $bin" >&2; exit 2; }
  cp "$bin" "$OUT/drules_proto_$suffix.bin"
  sha256sum "$OUT/drules_proto_$suffix.bin"
done
# Fail closed: every shipped binary must parse as a drape ContainerProto with
# at least one draw element (catches truncation/corruption, not aesthetics).
COMAPS_SRC="$COMAPS_SRC" "$VENV" - "$OUT" <<'PYEOF'
import glob
import os
import sys

sys.path.insert(0, os.path.join(os.environ.get("COMAPS_SRC", "/data/dev/src/comaps-4587b8ad"), "tools", "kothic", "src"))
from drules_struct_pb2 import ContainerProto

out = sys.argv[1]
count = 0
for path in sorted(glob.glob(os.path.join(out, "drules_proto_*.bin"))):
    cont = ContainerProto()
    with open(path, "rb") as f:
        cont.ParseFromString(f.read())
    elements = sum(len(c.element) for c in cont.cont)
    if elements == 0:
        raise SystemExit("empty drules container: {}".format(path))
    count += 1
    print("verified {}: {} draw elements".format(os.path.basename(path), elements))
if count != 4:
    raise SystemExit("expected 4 drules binaries, found {}".format(count))
PYEOF
echo "Abastevo style built into $OUT"
