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

ROOT="${MOBILE_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
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

# 5. No-background-tracking assertion (P16-T04, B-BR-L02/L04): the
# location-integrity path is one-shot only. Continuous-update APIs,
# background location permission and background-update flags must not
# appear in shipped location sources; freshness is enforced by the
# frozen contract, not by polling. Static only; energy/latency device
# matrices stay release-horizon and are never claimed here.
if grep -rn "ACCESS_BACKGROUND_LOCATION" "$ROOT/app/src/main" "$ROOT/data/src/main" 2>/dev/null; then
    fail "background location permission found in shipped sources"
fi
if grep -rn "requestLocationUpdates\|requestUpdates(" "$ROOT/data/src/main" "$ROOT/app/src/main" 2>/dev/null; then
    fail "continuous location polling found in shipped sources (one-shot only)"
fi
if grep -rn "startUpdatingLocation\|allowsBackgroundLocationUpdates" "$ROOT/iosApp/Sources" 2>/dev/null; then
    fail "background location tracking found in shipped iOS sources (one-shot only)"
fi
pass "no background location tracking in shipped sources"

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
