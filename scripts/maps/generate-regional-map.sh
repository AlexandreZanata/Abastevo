#!/usr/bin/env bash
# Reproducible CoMaps regional generation from one admitted local PBF.
# Build-time only: no network, database, Android or publication.
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: generate-regional-map.sh \
  --generator PATH --osmconvert PATH --input FILE.osm.pbf \
  --work DIR --output DIR --region ID --data DIR [options]

Required:
  --generator   pinned CoMaps generator_tool
  --osmconvert  osmconvert built from tools/osmctools
  --input       admitted local .osm.pbf
  --work        disposable working directory
  --output      package output directory
  --region      CoMaps country leaf ID (e.g. Brazil_Parana_East)
  --data        CoMaps data directory (borders, styles, countries.txt)

Options:
  --osmfilter PATH   osmfilter binary (required with --drop)
  --drop EXPR        explicit osmfilter drop expression; recorded verbatim
  --world-roads FILE optional world_roads file for multi-region routing
  --borders GLOB     border polygons copied into the work dir (default
                     "Brazil_*"). Every copied polygon enters the generator's
                     affiliation index; the full 1,148-polygon world set costs
                     minutes per run and writes features for unused regions.
  --timeout SECONDS  per-stage timeout (default 900)
  --replace          atomically replace an existing output package
  --help
USAGE
}

die() { echo "generate-regional-map: $*" >&2; exit 2; }
need() { [[ -n "${1:-}" ]] || die "missing $2"; }
file() { [[ -f "$1" ]] || die "not a file: $1"; }
exe() { [[ -x "$1" ]] || die "not executable: $1"; }

GENERATOR="" OSMCONVERT="" OSMTFILTER="" INPUT="" WORK="" OUTPUT="" REGION="" DATA="" DROP="" WORLD_ROADS="" TIMEOUT=900 REPLACE=0 BORDERS="Brazil_*"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --generator) GENERATOR="${2:-}"; shift 2 ;;
    --osmconvert) OSMCONVERT="${2:-}"; shift 2 ;;
    --osmfilter) OSMTFILTER="${2:-}"; shift 2 ;;
    --input) INPUT="${2:-}"; shift 2 ;;
    --work) WORK="${2:-}"; shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --region) REGION="${2:-}"; shift 2 ;;
    --data) DATA="${2:-}"; shift 2 ;;
    --drop) DROP="${2:-}"; shift 2 ;;
    --world-roads) WORLD_ROADS="${2:-}"; shift 2 ;;
    --borders) BORDERS="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    --replace) REPLACE=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

need "$GENERATOR" --generator; need "$OSMCONVERT" --osmconvert; need "$INPUT" --input
need "$WORK" --work; need "$OUTPUT" --output; need "$REGION" --region; need "$DATA" --data
[[ "$TIMEOUT" =~ ^[1-9][0-9]*$ ]] || die "timeout must be a positive integer"
exe "$GENERATOR"; exe "$OSMCONVERT"; file "$INPUT"; [[ -d "$DATA/borders" ]] || die "missing $DATA/borders"
if [[ -n "$DROP" ]]; then need "$OSMTFILTER" --osmfilter; exe "$OSMTFILTER"; fi
if [[ -n "$WORLD_ROADS" ]]; then file "$WORLD_ROADS"; fi
[[ "$REGION" =~ ^[A-Za-z0-9_()\ -]+$ ]] || die "unsafe region id"
DEST="$OUTPUT/$REGION.mwm"
if [[ -e "$DEST" && "$REPLACE" != "1" ]]; then die "output exists; pass --replace"; fi
run() { timeout --foreground "${TIMEOUT}s" "$@"; }
mkdir -p "$WORK/intermediate" "$WORK/cache" "$WORK/mwm" "$OUTPUT"
[[ "$BORDERS" =~ ^[A-Za-z0-9_*\ -]+$ ]] || die "unsafe borders glob"
[[ -f "$DATA/borders/$REGION.poly" ]] || die "no border polygon for $REGION"
if [[ ! -d "$WORK/mwm/borders" ]]; then
  mkdir -p "$WORK/mwm/borders.tmp"
  shopt -s nullglob
  # shellcheck disable=SC2206 # the glob must expand; names with spaces stay whole
  POLYS=("$DATA/borders/"$BORDERS.poly)
  shopt -u nullglob
  [[ ${#POLYS[@]} -gt 0 ]] || die "borders glob matched nothing: $BORDERS"
  cp "${POLYS[@]}" "$WORK/mwm/borders.tmp/"
  [[ -f "$WORK/mwm/borders.tmp/$REGION.poly" ]] || die "borders glob excludes $REGION"
  mv "$WORK/mwm/borders.tmp" "$WORK/mwm/borders"
fi

SOURCE_O5M="$WORK/source.o5m"
GEN_INPUT="$SOURCE_O5M"
run "$OSMCONVERT" "$INPUT" -o="$SOURCE_O5M"
if [[ -n "$DROP" ]]; then
  GEN_INPUT="$WORK/filtered.o5m"
  run "$OSMTFILTER" "$SOURCE_O5M" --drop="$DROP" -o="$GEN_INPUT"
fi

COMMON=(--osm_file_type=o5m --osm_file_name="$GEN_INPUT" --data_path="$WORK/mwm"
  --intermediate_data_path="$WORK/intermediate" --cache_path="$WORK/cache"
  --user_resource_path="$DATA")
run "$GENERATOR" "${COMMON[@]}" --preprocess
run "$GENERATOR" "${COMMON[@]}" --generate_features
run "$GENERATOR" "${COMMON[@]}" --generate_geometry --generate_index --output="$REGION"
run "$GENERATOR" "${COMMON[@]}" --generate_search_index --output="$REGION"
ROUTING=(--make_routing_index --generate_maxspeed --make_city_roads --make_cross_mwm --output="$REGION")
if [[ -n "$WORLD_ROADS" ]]; then ROUTING+=(--world_roads_path="$WORLD_ROADS"); fi
run "$GENERATOR" "${COMMON[@]}" "${ROUTING[@]}"

# check_mwm is a diagnostic, not an acceptance gate: upstream official packages
# fail it because it is stricter than the renderer/indexer. Engine-level render,
# search and an in-map route remain the acceptance checks.
CHECK_LOG="$OUTPUT/$REGION.check_mwm.log"
CHECK_EXIT=0
set +e
run "$GENERATOR" "${COMMON[@]}" --check_mwm --output="$REGION" >"$CHECK_LOG" 2>&1
CHECK_EXIT=$?
set -e
if [[ "$CHECK_EXIT" != "0" ]]; then
  echo "generate-regional-map: check_mwm diagnostic exit=$CHECK_EXIT (recorded in $CHECK_LOG)" >&2
fi

PACKAGE="$WORK/mwm/$REGION.mwm"
file "$PACKAGE"
TMP=$(mktemp "$OUTPUT/.${REGION}.XXXXXX")
cp "$PACKAGE" "$TMP"
chmod 0644 "$TMP"
mv -f "$TMP" "$DEST"
SHA=$(sha256sum "$DEST" | cut -d' ' -f1)
SIZE=$(stat -c '%s' "$DEST")
INPUT_SHA=$(sha256sum "$INPUT" | cut -d' ' -f1)
cat > "$OUTPUT/$REGION.generation.json" <<JSON
{
  "region": "$REGION",
  "generator": "$(basename "$GENERATOR")",
  "input_sha256": "$INPUT_SHA",
  "filter": "$DROP",
  "package": "$(basename "$DEST")",
  "bytes": $SIZE,
  "sha256": "$SHA",
  "check_mwm_exit": $CHECK_EXIT,
  "check_mwm_log": "$(basename "$CHECK_LOG")",
  "world_roads": "$WORLD_ROADS",
  "borders": "$BORDERS",
  "borders_count": $(find "$WORK/mwm/borders" -name '*.poly' | wc -l)
}
JSON
printf '{"region":"%s","bytes":%s,"sha256":"%s"}\n' "$REGION" "$SIZE" "$SHA"
