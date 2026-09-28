#!/usr/bin/env bash
# Phase 10.5 — builds and validates signed release APK + AAB.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MAX_APK_BYTES=$((15 * 1024 * 1024))

has_signing_config() {
    if [[ -f "$ROOT/signing.properties" ]]; then
        return 0
    fi
    if [[ -n "${ANPFUEL_KEYSTORE_PATH:-}" ]]; then
        return 0
    fi
    return 1
}

find_apksigner() {
    local sdk="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
    if [[ -z "$sdk" && -f "$ROOT/local.properties" ]]; then
        sdk="$(grep '^sdk.dir=' "$ROOT/local.properties" | cut -d= -f2- | tr -d '\r')"
    fi
    if [[ -z "$sdk" ]]; then
        echo "ERROR: ANDROID_HOME not set and sdk.dir missing in local.properties"
        exit 1
    fi
    local apksigner
    apksigner="$(find "$sdk/build-tools" -name apksigner -type f 2>/dev/null | sort -V | tail -n 1)"
    if [[ -z "$apksigner" ]]; then
        echo "ERROR: apksigner not found under $sdk/build-tools"
        exit 1
    fi
    echo "$apksigner"
}

if ! has_signing_config; then
    echo "ERROR: release signing not configured."
    echo "Copy signing.properties.example to signing.properties or set ANPFUEL_KEYSTORE_PATH."
    exit 1
fi

echo "Building signed release APK and AAB..."
./gradlew --no-daemon assembleRelease bundleRelease

APK_DIR="$ROOT/app/build/outputs/apk/release"
AAB_DIR="$ROOT/app/build/outputs/bundle/release"

APK="$(find "$APK_DIR" -maxdepth 1 -name '*.apk' -type f | head -n 1)"
AAB="$(find "$AAB_DIR" -maxdepth 1 -name '*.aab' -type f | head -n 1)"

if [[ -z "$APK" || ! -f "$APK" ]]; then
    echo "ERROR: release APK not found in $APK_DIR"
    exit 1
fi

if [[ -z "$AAB" || ! -f "$AAB" ]]; then
    echo "ERROR: release AAB not found in $AAB_DIR"
    exit 1
fi

APK_SIZE="$(stat -c%s "$APK" 2>/dev/null || stat -f%z "$APK")"
APK_MB="$(awk "BEGIN { printf \"%.2f\", $APK_SIZE / 1024 / 1024 }")"
echo "OK: release APK — ${APK_MB} MB ($APK_SIZE bytes) at $APK"

if (( APK_SIZE > MAX_APK_BYTES )); then
    echo "ERROR: release APK exceeds 15 MB limit"
    exit 1
fi

AAB_SIZE="$(stat -c%s "$AAB" 2>/dev/null || stat -f%z "$AAB")"
AAB_MB="$(awk "BEGIN { printf \"%.2f\", $AAB_SIZE / 1024 / 1024 }")"
echo "OK: release AAB — ${AAB_MB} MB ($AAB_SIZE bytes) at $AAB"

APKSIGNER="$(find_apksigner)"
echo "Verifying APK signature..."
"$APKSIGNER" verify --verbose "$APK"

echo "Verifying AAB signature..."
jarsigner -verify -verbose -certs "$AAB" | head -n 20

echo "Release build validation passed."
