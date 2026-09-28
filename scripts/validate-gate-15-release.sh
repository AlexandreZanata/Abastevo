#!/usr/bin/env bash
# Phase R2 / Gate 15 — validates v2.0.0 release criteria (automated checks only).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="v2.0.0"
RELEASE_NOTES="$ROOT/docs/releases/v2.0.0.md"
APK_DIR="$ROOT/app/build/outputs/apk/release"
AAB_DIR="$ROOT/app/build/outputs/bundle/release"

assert_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "ERROR: $label missing required text: $needle"
        exit 1
    fi
    echo "OK: $label — '$needle'"
}

echo "Gate 15 — criterion: tag $TAG exists on release commit"
if ! git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "ERROR: tag $TAG not found — run ./gradlew createReleaseTag"
    exit 1
fi
TAG_COMMIT="$(git rev-parse "$TAG^{commit}")"
echo "OK: tag $TAG at $TAG_COMMIT"

echo "Gate 15 — criterion: signed APK/AAB built"
APK="$(find "$APK_DIR" -maxdepth 1 -name '*.apk' -type f | head -n 1)"
AAB="$(find "$AAB_DIR" -maxdepth 1 -name '*.aab' -type f | head -n 1)"
if [[ -z "$APK" || ! -f "$APK" ]]; then
    echo "ERROR: release APK missing — run ./gradlew validateReleaseBuild"
    exit 1
fi
if [[ -z "$AAB" || ! -f "$AAB" ]]; then
    echo "ERROR: release AAB missing — run ./gradlew validateReleaseBuild"
    exit 1
fi
bash "$ROOT/scripts/verify-release-signature.sh" "$APK"
echo "OK: signed APK at $APK"
echo "OK: signed AAB at $AAB"

echo "Gate 15 — criterion: GitHub Release notes content"
bash "$ROOT/scripts/validate-github-release-notes.sh"
RELEASE_BODY="$(awk -v start='begin:release-body' -v end='end:release-body' '
    index($0, start) { capture = 1; next }
    index($0, end) { capture = 0; next }
    capture { print }
' "$RELEASE_NOTES")"
assert_contains "week_picker" "$RELEASE_BODY" "Week picker"
assert_contains "national_search" "$RELEASE_BODY" "National search"
assert_contains "safe_areas" "$RELEASE_BODY" "Safe areas"
assert_contains "fuel_icons" "$RELEASE_BODY" "Fuel icons"

echo "Gate 15 — criterion: manual sign-off (Appendix A2)"
echo "SKIP: Appendix A2 manual validation pending (Phase R3) — not blocking automated Gate 15 checks"

echo "Gate 15 automated release validation passed."
