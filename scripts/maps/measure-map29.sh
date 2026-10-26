#!/usr/bin/env bash
# MAP-29 local proxy receipt. Requires an owned emulator and an already-built debug/test APK.
# Usage:
#   SERIAL=emulator-5554 MAP_DIR=/path/to/three-mwm-files ./scripts/maps/measure-map29.sh
# Optional: APP_APK, TEST_APK, RELEASE_APK, OUT_DIR, APP_ID, TEST_PACKAGE, BUILD_TOOLS.
set -euo pipefail
SERIAL="${SERIAL:?set SERIAL to the owned emulator serial}"
MAP_DIR="${MAP_DIR:?set MAP_DIR to the directory containing Brazil_Parana_East.mwm, World.mwm and WorldCoasts.mwm}"
APP_APK="${APP_APK:-app/build/outputs/apk/debug/app-debug.apk}"
TEST_APK="${TEST_APK:-app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk}"
RELEASE_APK="${RELEASE_APK:-app/build/outputs/apk/release/app-release-unsigned.apk}"
OUT_DIR="${OUT_DIR:-docs/backend/evidence/maps/map29}"
APP_ID="${APP_ID:-com.anpfuel.app.debug}"
TEST_PACKAGE="${TEST_PACKAGE:-com.anpfuel.app.debug.test}"
BUILD_TOOLS="${BUILD_TOOLS:-${ANDROID_HOME:-/data/dev/android/sdk/Sdk}/build-tools/35.0.0}"
OUT="$OUT_DIR/raw-perf.txt"
STATIC="$OUT_DIR/static.txt"
mkdir -p "$OUT_DIR"

adb -s "$SERIAL" install -r "$APP_APK"
adb -s "$SERIAL" install -r "$TEST_APK"
for f in Brazil_Parana_East.mwm World.mwm WorldCoasts.mwm; do
  adb -s "$SERIAL" push "$MAP_DIR/$f" "/data/local/tmp/map29-$f" >/dev/null
  adb -s "$SERIAL" shell "run-as $APP_ID sh -c 'mkdir -p files/261004 && cp /data/local/tmp/map29-$f files/261004/$f'"
done

: > "$OUT"
{
  echo "# host $(uname -a)"
  echo "# date $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  adb -s "$SERIAL" shell getprop ro.build.fingerprint
  adb -s "$SERIAL" shell getconf PAGE_SIZE
  adb -s "$SERIAL" shell getprop ro.product.cpu.abi
  adb -s "$SERIAL" shell sha256sum /data/local/tmp/map29-Brazil_Parana_East.mwm /data/local/tmp/map29-World.mwm /data/local/tmp/map29-WorldCoasts.mwm
} >> "$OUT"

for i in 1 2 3; do
  echo "== PERF RUN $i" >> "$OUT"
  adb -s "$SERIAL" shell am force-stop "$APP_ID"
  adb -s "$SERIAL" logcat -c
  adb -s "$SERIAL" shell am instrument -w \
    -e class com.anpfuel.app.ui.map.MapPerformanceInstrumentedTest \
    "$TEST_PACKAGE/androidx.test.runner.AndroidJUnitRunner" 2>&1 | tail -4 >> "$OUT"
  adb -s "$SERIAL" logcat -d -s MapPerf >> "$OUT"
done

for i in 1 2 3; do
  echo "== COLD START $i" >> "$OUT"
  adb -s "$SERIAL" shell am force-stop "$APP_ID"
  sleep 2
  adb -s "$SERIAL" shell am start -W -n "$APP_ID/com.anpfuel.app.MainActivity" 2>&1 \
    | grep -E "Status|LaunchState|TotalTime|WaitTime|ThisTime" >> "$OUT"
done

if [[ -f "$RELEASE_APK" ]]; then
  {
    echo "# MAP-29 static compatibility evidence"
    echo "# date $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "# java"; java -version 2>&1 | sed 's/^/# /'
    echo "# apk"; stat -c '%s bytes' "$RELEASE_APK"; sha256sum "$RELEASE_APK"
    echo "# aapt"; "$BUILD_TOOLS/aapt" dump badging "$RELEASE_APK" | grep -E "package:|sdkVersion|targetSdkVersion|native-code"
    echo "# zipalign"; "$BUILD_TOOLS/zipalign" -c -P 16 4 "$RELEASE_APK" && echo PASS
    echo "# packaged native libs"
    unzip -l "$RELEASE_APK" | awk '/lib\/.*\.so$/ {print $4, $1}'
    echo "# no mwm assets"; unzip -l "$RELEASE_APK" | grep -Ei '\.mwm' || echo NONE
  } > "$STATIC"
  LIB_TMP="$(mktemp -d)"
  unzip -q "$RELEASE_APK" 'lib/*/*.so' -d "$LIB_TMP"
  for f in "$LIB_TMP"/lib/*/*.so; do
    echo "== $f" >> "$STATIC"
    readelf -lW "$f" | awk '/LOAD/ {print "align=" $NF}' >> "$STATIC"
  done
  rm -rf "$LIB_TMP"
fi

echo "wrote $OUT${RELEASE_APK:+ and $STATIC}"
