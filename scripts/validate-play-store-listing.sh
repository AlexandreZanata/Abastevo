#!/usr/bin/env bash
# Phase 10.4 — validates Play Store listing draft character limits and disclaimers.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LISTING="$ROOT/docs/play-store-listing.md"
PRIVACY="$ROOT/docs/privacy-policy.md"

if [[ ! -f "$LISTING" ]]; then
    echo "ERROR: missing $LISTING"
    exit 1
fi

if [[ ! -f "$PRIVACY" ]]; then
    echo "ERROR: missing $PRIVACY"
    exit 1
fi

extract_block() {
    local start_marker="$1"
    local end_marker="$2"
    awk -v start="$start_marker" -v end="$end_marker" '
        index($0, start) { capture = 1; next }
        index($0, end) { capture = 0; next }
        capture { print }
    ' "$LISTING" | sed '/^[[:space:]]*$/d'
}

assert_max_length() {
    local label="$1"
    local max="$2"
    local text="$3"
    local length="${#text}"
    if (( length > max )); then
        echo "ERROR: $label exceeds $max characters (actual: $length)"
        echo "$text"
        exit 1
    fi
    echo "OK: $label — $length / $max characters"
}

assert_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "ERROR: $label missing required text: $needle"
        exit 1
    fi
    echo "OK: $label contains '$needle'"
}

SHORT_EN="$(extract_block 'begin:short-description-en' 'end:short-description-en')"
SHORT_PT="$(extract_block 'begin:short-description-pt' 'end:short-description-pt')"
FULL_EN="$(extract_block 'begin:full-description-en' 'end:full-description-en')"
FULL_PT="$(extract_block 'begin:full-description-pt' 'end:full-description-pt')"

assert_max_length "short_description_en" 80 "$SHORT_EN"
assert_max_length "short_description_pt" 80 "$SHORT_PT"
assert_max_length "full_description_en" 4000 "$FULL_EN"
assert_max_length "full_description_pt" 4000 "$FULL_PT"

assert_contains "disclaimer_en" "$FULL_EN" "not affiliated"
assert_contains "attribution_en" "$FULL_EN" "ANP"
assert_contains "offline_en" "$FULL_EN" "offline"

assert_contains "disclaimer_pt" "$FULL_PT" "não é afiliado"
assert_contains "attribution_pt" "$FULL_PT" "ANP"
assert_contains "offline_pt" "$FULL_PT" "offline"

assert_contains "privacy_policy" "$(cat "$PRIVACY")" "does **not collect"

echo "Play Store listing validation passed."
