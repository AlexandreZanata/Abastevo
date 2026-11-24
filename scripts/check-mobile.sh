#!/usr/bin/env bash
# P12-T05 bounded mobile check selection for the KMP foundation phase.
# Static guards run everywhere (CI-safe, no JDK/Xcode): portable import bans,
# golden-fixture validity and the iosApp skeleton shape. Full mode additionally
# runs the Android baseline when JDK+SDK are present and the Swift suite when a
# Swift toolchain is present; absent toolchains report an explicit loud SKIP
# (SKIP is not evidence) instead of failing or faking green.
# Usage: scripts/check-mobile.sh [--static-only]
# Test override: MOBILE_ROOT=<dir> points at a fixture tree (see test-gate).
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="${MOBILE_ROOT:-$SCRIPT_ROOT}"
MODE="full"
if [[ "${1:-}" == "--static-only" ]]; then
    MODE="static"
elif [[ -n "${1:-}" ]]; then
    echo "ERROR: usage: check-mobile.sh [--static-only]" >&2
    exit 1
fi

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "OK: $1"; }

echo "== mobile static guards ($MODE) =="

# 1. Portable import bans: pure commonMain-ready code keeps zero platform deps.
for pkg in \
    "domain/src/main/kotlin/com/anpfuel/domain/portable" \
    "application/src/main/kotlin/com/anpfuel/application/portable"; do
    dir="$ROOT/$pkg"
    [[ -d "$dir" ]] || fail "missing portable dir: $pkg"
    [[ -n "$(ls "$dir"/*.kt 2>/dev/null)" ]] || fail "no Kotlin sources in $pkg"
    while IFS= read -r f; do
        while IFS= read -r line; do
            trimmed="$(echo "$line" | sed 's/^[[:space:]]*//')"
            case "$trimmed" in
                "import android."*|"import androidx."*|"import java."*|\
                "import javax."*|"import dagger."*|"import dagger "*|\
                "import com.google.dagger"*|"import kotlinx.coroutines"*)
                    fail "$f imports banned platform API: $trimmed"
                    ;;
            esac
        done < "$f"
    done < <(find "$dir" -name '*.kt')
    pass "no platform imports in $pkg"
done

# 2. Golden fixture validity plus key-vector presence (mirrors parity tests).
FIXTURE="$ROOT/contracts/testdata/compat/money-portable-v1.json"
[[ -f "$FIXTURE" ]] || fail "missing golden fixture contracts/testdata/compat/money-portable-v1.json"
python3 - "$FIXTURE" <<'PY' || fail "golden fixture invalid or missing key vectors"
import json, sys
body = open(sys.argv[1]).read()
doc = json.loads(body)
assert doc.get("id") == "money-portable-v1", "fixture id"
assert doc.get("min_milli_brl") == 1 and doc.get("max_milli_brl") == 1000000, "bounds"
for needle in ['"text": "5,999"', '"milli": 5999', '"text": "5,9999"',
               '"code": "over-precision"', '"total_milli": 274950',
               '"text_max_scalars": 280', "123e4567-e89b-12d3-a456-426614174000"]:
    assert needle in body, needle
print("fixture vectors ok")
PY
pass "golden fixture valid with key vectors"

# 3. iosApp skeleton shape plus force-unwrap heuristic in the core target.
for f in "iosApp/Package.swift" \
    "iosApp/Sources/AnpFuelCore/PortableMoney.swift" \
    "iosApp/Sources/AnpFuelCore/PortableText.swift" \
    "iosApp/Sources/AnpFuelCore/PortableIdTime.swift" \
    "iosApp/Sources/AnpFuelCore/TankFillUseCase.swift" \
    "iosApp/Sources/AnpFuelShell/AnpFuelApp.swift" \
    "iosApp/Tests/AnpFuelCoreTests/PortableMoneyTests.swift" \
    "iosApp/Tests/AnpFuelCoreTests/PortableTextTests.swift" \
    "iosApp/Tests/AnpFuelCoreTests/PortableIdTimeTests.swift" \
    "iosApp/Tests/AnpFuelCoreTests/TankFillUseCaseTests.swift"; do
    [[ -f "$ROOT/$f" ]] || fail "missing $f"
done
pass "iosApp skeleton files present"
python3 - "$ROOT/iosApp/Sources/AnpFuelCore" <<'PY' || fail "force-unwrap found in AnpFuelCore"
import os, re, sys
root = sys.argv[1]
hits = []
for name in sorted(os.listdir(root)):
    if not name.endswith(".swift"):
        continue
    for n, line in enumerate(open(os.path.join(root, name)), 1):
        for m in re.finditer(r"[A-Za-z0-9_\)\]]!(?![=])", line):
            # '!=' already excluded by lookahead; prefix '!' has no alnum/paren before.
            hits.append(f"{name}:{n}:{line.strip()}")
assert not hits, "postfix unwrap: " + "; ".join(hits)
print("no force-unwrap in AnpFuelCore")
PY
pass "no force-unwrap in AnpFuelCore"

# 4. Release artifact assertion (P16-T02): no test-injection hook in
# production sources. Debug injection lives only behind the
# LocationEnvironment port (BuildConfig.DEBUG) and in test fakes;
# setting testInjected=true in shipped code would smuggle an
# always-simulated path into release. Static only; device run still
# proves the provider wiring on hardware.
if grep -rn "testInjected *= *true" "$ROOT/data/src/main" "$ROOT/app/src/main" 2>/dev/null; then
    fail "test-injection hook found in production sources"
fi
pass "no test-injection hooks in release sources"

# 5. No-background-tracking assertion (P16-T04, B-BR-L02/L04; EMP-GPS-002 EDR-323).
# Browsing and route preview use one-shot requests. Only the four explicitly
# started location foreground services may subscribe to updates. Guidance can
# use high-accuracy fused location or GPS, and the two recorders use GPS. The
# organization trip tracking service (owner decision EDR-271) has one pinned
# subscription, starts only after consuming the visible trip screen's grant,
# is never sticky, and no boot receiver or background permission exists.
[[ -d "$ROOT/app/src/main" && -d "$ROOT/data/src/main" ]] || fail "missing Android location guard source roots"
if grep -rn "ACCESS_BACKGROUND_LOCATION" "$ROOT/app/src/main" "$ROOT/data/src/main" 2>/dev/null; then
    fail "background location permission found in shipped sources"
fi
if grep -rn "requestLocationUpdates\|requestUpdates(" "$ROOT/data/src/main" "$ROOT/app/src/main" 2>/dev/null | grep -v "app/src/main/kotlin/com/anpfuel/app/ui/map/GuidanceService.kt" | grep -v "app/src/main/kotlin/com/anpfuel/app/ui/map/TripRecordingService.kt" | grep -v "app/src/main/kotlin/com/anpfuel/app/ui/driver/work/DriverGpsService.kt" | grep -v "app/src/main/kotlin/com/anpfuel/app/ui/enterprise/tracking/OrganizationTrackingService.kt"; then
    fail "continuous location polling found outside the permitted location foreground services (one-shot only elsewhere)"
fi
guidance="$ROOT/app/src/main/kotlin/com/anpfuel/app/ui/map/GuidanceService.kt"
if grep -nE 'requestLocationUpdates|requestUpdates\(' "$guidance" | grep -Ev '^[0-9]+:[[:space:]]*LocationManagerCompat\.requestLocationUpdates\(manager, LocationManager\.(FUSED|GPS)_PROVIDER, request, active, Looper\.getMainLooper\(\)\)$'; then
    fail "unexpected continuous location source in GuidanceService"
fi
if ! grep -Fq 'val request = LocationRequestCompat.Builder(1_000L)' "$guidance" ||
   ! grep -Fq '.setQuality(LocationRequestCompat.QUALITY_HIGH_ACCURACY)' "$guidance"; then
    fail "GuidanceService must request one-second high-accuracy location updates"
fi
guidance_block=$(awk '/GuidanceService/ {found=1} found {print} found && /\/>/ {exit}' "$ROOT/app/src/main/AndroidManifest.xml")
if ! grep -q 'android:exported="false"' <<<"$guidance_block" || ! grep -q 'foregroundServiceType="location"' <<<"$guidance_block"; then
    fail "GuidanceService must stay a non-exported location foreground service"
fi
if grep -rn "requestLocationUpdates" "$ROOT/app/src/main/kotlin/com/anpfuel/app/ui/map/TripRecordingService.kt" 2>/dev/null | grep -v "GPS_PROVIDER, 2000L" | grep -v "removeUpdates"; then
    fail "unexpected continuous location source in TripRecordingService"
fi
if grep -rn "requestLocationUpdates" "$ROOT/app/src/main/kotlin/com/anpfuel/app/ui/driver/work/DriverGpsService.kt" 2>/dev/null | grep -v "GPS_PROVIDER, 2000L" | grep -v "removeUpdates"; then
    fail "unexpected continuous location source in DriverGpsService"
fi
driver_gps_block=$(awk '/DriverGpsService/ {found=1} found {print} found && /\/>/ {exit}' "$ROOT/app/src/main/AndroidManifest.xml")
if ! grep -q 'foregroundServiceType="location"' <<<"$driver_gps_block"; then
    fail "DriverGpsService must stay a location foreground service"
fi
trip_block=$(awk '/TripRecordingService/ {found=1} found {print} found && /\/>/ {exit}' "$ROOT/app/src/main/AndroidManifest.xml")
if ! grep -q 'android:exported="false"' <<<"$trip_block" || ! grep -q 'foregroundServiceType="location"' <<<"$trip_block"; then
    fail "TripRecordingService must stay a non-exported location foreground service"
fi
if grep -rn "BOOT_COMPLETED\|LOCKED_BOOT_COMPLETED" "$ROOT/app/src/main" "$ROOT/data/src/main" 2>/dev/null; then
    fail "boot receiver found in shipped sources (no location work may start at boot)"
fi
tracking="$ROOT/app/src/main/kotlin/com/anpfuel/app/ui/enterprise/tracking/OrganizationTrackingService.kt"
tracking_block=$(awk '/OrganizationTrackingService"/ {found=1} found {print} found && /\/>/ {exit}' "$ROOT/app/src/main/AndroidManifest.xml")
if [[ -n "$tracking_block" || -f "$tracking" ]]; then
    [[ -f "$tracking" ]] || fail "OrganizationTrackingService is declared without its pinned source"
    if grep -nE 'requestLocationUpdates|requestUpdates\(' "$tracking" | grep -Ev '^[0-9]+:[[:space:]]*LocationManagerCompat\.requestLocationUpdates\(manager, provider, request, listener, Looper\.getMainLooper\(\)\)$'; then
        fail "unexpected continuous location source in OrganizationTrackingService"
    fi
    if ! grep -Fq 'val provider = if (precision == LocationPermission.PRECISE) LocationManager.GPS_PROVIDER else coarseProvider(manager)' "$tracking" ||
       ! grep -Fq 'val request = LocationRequestCompat.Builder(5_000L).setMinUpdateIntervalMillis(5_000L)' "$tracking"; then
        fail "OrganizationTrackingService must request five-second GPS (precise) or fused/network (approximate) updates"
    fi
    if grep -nE 'START_STICKY|START_REDELIVER_INTENT' "$tracking" || ! grep -Fq 'return START_NOT_STICKY' "$tracking"; then
        fail "OrganizationTrackingService must never be restarted by the system"
    fi
    if ! grep -Fq 'val granted = gate.consume(partition, sessionId)' "$ROOT/application/src/main/kotlin/com/anpfuel/application/enterprise/tracking/TrackingCollector.kt" 2>/dev/null; then
        fail "organization tracking must consume the visible trip screen's start grant"
    fi
    if ! grep -q 'android:exported="false"' <<<"$tracking_block" || ! grep -q 'foregroundServiceType="location"' <<<"$tracking_block" ||
       grep -q 'android:exported="true"' <<<"$tracking_block"; then
        fail "OrganizationTrackingService must stay a non-exported location foreground service"
    fi
fi
if grep -rn "startUpdatingLocation\|allowsBackgroundLocationUpdates" "$ROOT/iosApp/Sources" 2>/dev/null; then
    fail "background location tracking found in shipped iOS sources (one-shot only)"
fi
pass "no background location tracking in shipped sources"

# MOBILE_ROOT may point to a synthetic tree for the shell guards. The Python
# suites require the complete repository and run when checking the real tree.
if [[ "$ROOT" -ef "$SCRIPT_ROOT" ]]; then
    (
        cd "$ROOT"
        python3 -m unittest -v scripts.tests.test_ios_app_integrity scripts.tests.test_ios_localization
    ) || fail "iOS integrity or localization checks failed"
    pass "iOS project integrity and localization catalogs"
fi

if [[ "$MODE" == "static" ]]; then
    echo "check-mobile: static-only ok"
    exit 0
fi

echo "== mobile full mode (toolchain-gated) =="
if command -v java >/dev/null 2>&1 && [[ -n "${ANDROID_HOME:-}" || -d "$ROOT/local.properties" || -d "$HOME/Android/Sdk" ]]; then
    echo "-- Android baseline --"
    (cd "$ROOT" && ./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon) \
        || fail "Android baseline failed"
    pass "Android baseline green"
else
    echo "SKIP: Android baseline needs JDK + SDK (absent here); run it at task level/phase exit — SKIP is not evidence"
fi

if command -v swift >/dev/null 2>&1; then
    echo "-- Swift suite --"
    (cd "$ROOT/iosApp" && swift test) || fail "Swift suite failed"
    pass "Swift suite green"
else
    echo "SKIP: Swift toolchain absent (macOS + Xcode 26.4 required); BLOCKED until Mac access — SKIP is not evidence"
fi

echo "check-mobile: full ok (executed checks passed; SKIPs above are outstanding work, not green)"
