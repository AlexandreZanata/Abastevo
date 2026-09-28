#!/usr/bin/env bash
# Phase 15 — validates GitHub Release v2.0.0 draft notes and required disclaimers.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RELEASE_NOTES="$ROOT/docs/releases/v2.0.0.md"

if [[ ! -f "$RELEASE_NOTES" ]]; then
    echo "ERROR: missing $RELEASE_NOTES"
    exit 1
fi

extract_block() {
    local start_marker="$1"
    local end_marker="$2"
    awk -v start="$start_marker" -v end="$end_marker" '
        index($0, start) { capture = 1; next }
        index($0, end) { capture = 0; next }
        capture { print }
    ' "$RELEASE_NOTES"
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

assert_not_empty() {
    local label="$1"
    local text="$2"
    if [[ -z "${text//[[:space:]]/}" ]]; then
        echo "ERROR: $label is empty"
        exit 1
    fi
    echo "OK: $label is present"
}

RELEASE_BODY="$(extract_block 'begin:release-body' 'end:release-body')"
FULL_DOC="$(cat "$RELEASE_NOTES")"

assert_not_empty "release_body" "$RELEASE_BODY"
assert_contains "version" "$FULL_DOC" "v2.0.0"
assert_contains "anp_attribution" "$RELEASE_BODY" "ANP"
assert_contains "not_affiliated" "$RELEASE_BODY" "not affiliated with ANP"
assert_contains "week_picker" "$RELEASE_BODY" "Week picker"
assert_contains "install_apk" "$RELEASE_BODY" "app-release.apk"
assert_contains "privacy_policy" "$RELEASE_BODY" "privacy policy"
assert_contains "mit_license" "$RELEASE_BODY" "MIT License"
assert_contains "icon_attribution" "$RELEASE_BODY" "Pictogrammers"
assert_contains "repository_url" "$RELEASE_BODY" "github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS"
assert_contains "br009_checklist" "$FULL_DOC" "BR-009"

echo "GitHub Release v2.0.0 notes validation passed."
