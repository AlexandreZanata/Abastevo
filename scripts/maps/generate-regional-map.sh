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
  --replace          atomically replace an existing output package
  --help
USAGE
}

die() { echo "generate-regional-map: $*" >&2; exit 2; }
need() { [[ -n "${1:-}" ]] || die "missing $2"; }
file() { [[ -f "$1" ]] || die "not a file: $1"; }
exe() { [[ -x "$1" ]] || die "not executable: $1"; }

GENERATOR="" OSMCONVERT="" OSMTFILTER="" INPUT="" WORK="" OUTPUT="" REGION="" DATA="" DROP="" REPLACE=0
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
    --replace) REPLACE=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

need "$GENERATOR" --generator; need "$OSMCONVERT" --osmconvert; need "$INPUT" --input
need "$WORK" --work; need "$OUTPUT" --output; need "$REGION" --region; need "$DATA" --data
exe "$GENERATOR"; exe "$OSMCONVERT"; file "$INPUT"; [[ -d "$DATA/borders" ]] || die "missing $DATA/borders"
if [[ -n "$DROP" ]]; then need "$OSMTFILTER" --osmfilter; exe "$OSMTFILTER"; fi
[[ "$REGION" =~ ^[A-Za-z0-9_()\ -]+$ ]] || die "unsafe region id"
mkdir -p "$WORK/intermediate" "$WORK/cache" "$WORK/mwm" "$OUTPUT"
if [[ ! -d "$WORK/mwm/borders" ]]; then cp -r "$DATA/borders" "$WORK/mwm/borders"; fi

SOURCE_O5M="$WORK/source.o5m"
GEN_INPUT="$SOURCE_O5M"
"$OSMCONVERT" "$INPUT" -o="$SOURCE_O5M"
if [[ -n "$DROP" ]]; then
  GEN_INPUT="$WORK/filtered.o5m"
  "$OSMTFILTER" "$SOURCE_O5M" --drop="$DROP" -o="$GEN_INPUT"
fi

COMMON=(--osm_file_type=o5m --osm_file_name="$GEN_INPUT" --data_path="$WORK/mwm"
  --intermediate_data_path="$WORK/intermediate" --cache_path="$WORK/cache"
  --user_resource_path="$DATA")
"$GENERATOR" "${COMMON[@]}" --preprocess
"$GENERATOR" "${COMMON[@]}" --generate_features
"$GENERATOR" "${COMMON[@]}" --generate_geometry --generate_index --output="$REGION"
"$GENERATOR" "${COMMON[@]}" --generate_search_index --output="$REGION"
"$GENERATOR" "${COMMON[@]}" --make_routing_index --generate_maxspeed --make_city_roads --output="$REGION"
"$GENERATOR" "${COMMON[@]}" --check_mwm --output="$REGION"

PACKAGE="$WORK/mwm/$REGION.mwm"
file "$PACKAGE"
DEST="$OUTPUT/$REGION.mwm"
if [[ -e "$DEST" && "$REPLACE" != "1" ]]; then die "output exists; pass --replace"; fi
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
  "sha256": "$SHA"
}
JSON
printf '{"region":"%s","bytes":%s,"sha256":"%s"}\n' "$REGION" "$SIZE" "$SHA"
