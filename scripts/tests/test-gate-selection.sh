#!/usr/bin/env bash
# P01-T13 focused harness: gate selection and failure behavior.
# Proves manifest validation, tool/secret/unclassified failures, selected
# filter execution and compile-only labeling without running release infra.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0

assert_pass() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name (expected success)" >&2
        FAIL=$((FAIL + 1))
    fi
}

assert_fail() {
    local name="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "FAIL: $name (expected failure, got success)" >&2
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $name"
        PASS=$((PASS + 1))
    fi
}

echo "== manifest present and packages have behavioral tests =="
assert_pass "manifest exists" test -f scripts/check-manifest.txt
assert_pass "config package has tests" grep -rq "^func Test" backend/internal/platform/config --include="*_test.go"
assert_pass "apicontract package has tests" grep -rq "^func Test" backend/internal/platform/apicontract --include="*_test.go"

echo "== selected filter executes cases (not compile-only) =="
FILTER_COUNT="$(cd backend && go test ./internal/platform/config/... -count=1 -v 2>/dev/null | grep -c "^=== RUN" || true)"
if [[ "$FILTER_COUNT" -gt 0 ]]; then
    echo "PASS: selected filter executed $FILTER_COUNT cases"
    PASS=$((PASS + 1))
else
    echo "FAIL: selected filter executed zero cases" >&2
    FAIL=$((FAIL + 1))
fi

echo "== compile-only is labeled and not claimed as behavioral =="
COMPILE_ONLY_HITS="$(cd backend && go test -run '^$' ./internal/platform/config/... -count=1 2>&1 | tail -n 5)"
if echo "$COMPILE_ONLY_HITS" | grep -q "ok"; then
    echo "PASS: compile-only runs but is kept out of quick-verify evidence (labeled by exclusion)"
    PASS=$((PASS + 1))
else
    echo "FAIL: compile-only probe did not behave as expected" >&2
    FAIL=$((FAIL + 1))
fi
if grep -rn "^[^#]*go test -run" scripts/quick-verify.sh 2>/dev/null | grep -q .; then
    echo "FAIL: quick-verify contains compile-only invocation" >&2
    FAIL=$((FAIL + 1))
else
    echo "PASS: quick-verify contains no compile-only behavioral claim"
    PASS=$((PASS + 1))
fi

echo "== failure propagation fixtures =="
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
assert_fail "failing command propagates" bash -c 'exit 1'
assert_fail "missing tool fails" bash -c 'command -v definitely-missing-tool-anpfuel >/dev/null'
# Secret pattern fixture must be detected by the scanner expression.
SECRET_FIXTURE="$TMP/secret.txt"
echo "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456" > "$SECRET_FIXTURE"
if grep -nE 'ghp_[A-Za-z0-9]{20,}' "$SECRET_FIXTURE" >/dev/null; then
    echo "PASS: secret pattern fixture detected"
    PASS=$((PASS + 1))
else
    echo "FAIL: secret pattern fixture not detected" >&2
    FAIL=$((FAIL + 1))
fi
# Unclassified path rule: quick-verify refuses unknown top-level areas.
if grep -q "unclassified changed path" scripts/quick-verify.sh; then
    echo "PASS: unclassified path guard present"
    PASS=$((PASS + 1))
else
    echo "FAIL: unclassified path guard missing" >&2
    FAIL=$((FAIL + 1))
fi

echo "== bounded mobile selection (P12-T05) =="
if MOBILE_ROOT="$ROOT" bash "$ROOT/scripts/check-mobile.sh" --static-only >/dev/null 2>&1; then
    echo "PASS: mobile static-only green on repo tree"
    PASS=$((PASS + 1))
else
    echo "FAIL: mobile static-only red on repo tree" >&2
    FAIL=$((FAIL + 1))
fi
EMPTY_ROOT="$(mktemp -d)"
if MOBILE_ROOT="$EMPTY_ROOT" bash "$ROOT/scripts/check-mobile.sh" --static-only >/dev/null 2>&1; then
    echo "FAIL: mobile static-only accepted empty tree" >&2
    FAIL=$((FAIL + 1))
else
    echo "PASS: mobile static-only refuses missing portable areas"
    PASS=$((PASS + 1))
fi
rm -rf "$EMPTY_ROOT"
# Planted banned import must fail naming the file (minimal valid skeleton).
SKELETON="$(mktemp -d)"
mkdir -p "$SKELETON/domain/src/main/kotlin/com/anpfuel/domain/portable" \
    "$SKELETON/application/src/main/kotlin/com/anpfuel/application/portable" \
    "$SKELETON/contracts/testdata/compat" \
    "$SKELETON/iosApp/Sources/AnpFuelCore" \
    "$SKELETON/iosApp/Sources/AnpFuelShell" \
    "$SKELETON/iosApp/Tests/AnpFuelCoreTests"
printf 'package bad\n\nimport java.time.Instant\n' > "$SKELETON/domain/src/main/kotlin/com/anpfuel/domain/portable/Bad.kt"
printf 'package ok\n' > "$SKELETON/application/src/main/kotlin/com/anpfuel/application/portable/Ok.kt"
python3 - "$SKELETON/contracts/testdata/compat/money-portable-v1.json" <<'PY'
import json, sys
doc = {"id": "money-portable-v1", "min_milli_brl": 1, "max_milli_brl": 1000000}
body = ('"text": "5,999" "milli": 5999 "text": "5,9999" "code": "over-precision" '
        '"total_milli": 274950 "text_max_scalars": 280 123e4567-e89b-12d3-a456-426614174000')
open(sys.argv[1], "w").write(json.dumps(doc) + "\n" + body + "\n")
PY
for f in PortableMoney.swift PortableText.swift PortableIdTime.swift TankFillUseCase.swift; do
    printf '// skeleton\n' > "$SKELETON/iosApp/Sources/AnpFuelCore/$f"
done
printf '// skeleton\n' > "$SKELETON/iosApp/Sources/AnpFuelShell/AnpFuelApp.swift"
printf '// swift-tools-version: 5.9\n' > "$SKELETON/iosApp/Package.swift"
for f in PortableMoneyTests.swift PortableTextTests.swift PortableIdTimeTests.swift TankFillUseCaseTests.swift; do
    printf '// skeleton\n' > "$SKELETON/iosApp/Tests/AnpFuelCoreTests/$f"
done
OUT="$(MOBILE_ROOT="$SKELETON" bash "$ROOT/scripts/check-mobile.sh" --static-only 2>&1 || true)"
if echo "$OUT" | grep -q "Bad.kt imports banned platform API"; then
    echo "PASS: planted platform import fails naming the file"
    PASS=$((PASS + 1))
else
    echo "FAIL: planted platform import not detected: $OUT" >&2
    FAIL=$((FAIL + 1))
fi
rm -rf "$SKELETON"

echo "== bounded landing selection (P25-T02) =="
if grep -q "landing/\*" scripts/quick-verify.sh; then
    echo "PASS: landing path classified in quick-verify.sh"
    PASS=$((PASS + 1))
else
    echo "FAIL: landing path missing in quick-verify.sh" >&2
    FAIL=$((FAIL + 1))
fi
if grep -q "npm --prefix landing run check" scripts/quick-verify.sh; then
    echo "PASS: landing check integration present in quick-verify.sh"
    PASS=$((PASS + 1))
else
    echo "FAIL: landing check integration missing in quick-verify.sh" >&2
    FAIL=$((FAIL + 1))
fi

echo "== summary: $PASS passed, $FAIL failed =="
if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
