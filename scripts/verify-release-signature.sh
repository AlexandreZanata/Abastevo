#!/usr/bin/env bash
# Verifies APK signature using apksigner from the Android SDK.
set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <path-to.apk>"
    exit 1
fi

APK="$1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_PROPS="$ROOT/local.properties"

if [[ ! -f "$APK" ]]; then
    echo "ERROR: APK not found: $APK"
    exit 1
fi

if [[ ! -f "$LOCAL_PROPS" ]]; then
    echo "ERROR: local.properties not found (sdk.dir required)"
    exit 1
fi

SDK_DIR="$(grep -E '^sdk\.dir=' "$LOCAL_PROPS" | cut -d= -f2- | sed 's/\\:/:/g')"
BUILD_TOOLS="$(ls -1d "$SDK_DIR/build-tools"/* 2>/dev/null | sort -V | tail -1)"
APKSIGNER="$BUILD_TOOLS/apksigner"

if [[ ! -x "$APKSIGNER" ]]; then
    echo "ERROR: apksigner not found under $BUILD_TOOLS"
    exit 1
fi

echo "Verifying signature: $APK"
"$APKSIGNER" verify --verbose "$APK"

echo "APK signature verification passed."
