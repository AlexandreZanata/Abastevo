#!/usr/bin/env bash
# P01-T13 quick verification: bounded integration gate shared locally/remotely.
# Composes existing gates instead of duplicating logic in YAML.
# Fails loudly; never hides tool/parser errors. Docs-only changes run a
# meaningful docs + contract subset; code changes run the full quick set.
# Unclassified paths, missing tools and secrets in working tree or committed
# PR diff all fail. Compile-only `go test -run '^$'` is never claimed here
# as behavioral evidence.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

START_EPOCH="$(date +%s)"
MANIFEST="$ROOT/scripts/check-manifest.txt"

require_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "ERROR: missing tool $1 ($2)" >&2
        exit 1
    fi
}

require_tool go "toolchain go1.27.1 via backend/go.mod"
require_tool sqlc "go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1"
require_tool staticcheck "GOTOOLCHAIN=go1.27.1 go install honnef.co/go/tools/cmd/staticcheck@v0.8.1"
require_tool vacuum "go install github.com/daveshanley/vacuum@v0.30.6"
require_tool govulncheck "GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@v1.8.0"

if [[ ! -f "$MANIFEST" ]]; then
    echo "ERROR: missing check manifest at scripts/check-manifest.txt" >&2
    exit 1
fi

# Validate manifest quick packages: each must exist and contain at least one
# real Go test. A missing package or zero matched tests fails explicitly.
echo "== manifest validation =="
in_packages=0
while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="$(echo "$line" | tr -d '[:space:]')"
    if [[ -z "$line" ]]; then
        continue
    fi
    if [[ "$line" == "[quick-packages]" ]]; then
        in_packages=1
        continue
    fi
    if [[ "$line" == "["* ]]; then
        in_packages=0
        continue
    fi
    if [[ "$in_packages" == "1" ]]; then
        if [[ ! -d "$ROOT/$line" ]]; then
            echo "ERROR: manifest package missing: $line" >&2
            exit 1
        fi
        if ! grep -rq "^func Test" "$ROOT/$line" --include="*_test.go"; then
            echo "ERROR: manifest package has zero behavioral tests: $line" >&2
            exit 1
        fi
        echo "OK: $line"
    fi
done < "$MANIFEST"

# Collect changed files from working tree (tracked + untracked) and, when
# available, the committed PR diff against origin/main. CI checkouts are clean,
# so working-tree status alone would scan nothing; the PR diff covers that case.
CHANGED_WORKTREE="$(git status --porcelain -- . 2>/dev/null | awk '{print $2}' || true)"
CHANGED_DIFF=""
if git rev-parse --verify origin/main >/dev/null 2>&1; then
    MERGE_BASE="$(git merge-base HEAD origin/main 2>/dev/null || true)"
    if [[ -n "$MERGE_BASE" ]]; then
        CHANGED_DIFF="$(git diff --name-only "$MERGE_BASE"...HEAD 2>/dev/null || true)"
    fi
fi
CHANGED_ALL="$(printf "%s\n%s" "$CHANGED_WORKTREE" "$CHANGED_DIFF" | sort -u | grep -v '^$' || true)"

# Classify selection. Docs paths run the docs subset; backend/contract/infra/
# gate/mobile paths run the full quick set; any other tracked path is
# unclassified and fails so new areas cannot silently bypass selection.
# Mobile areas (P12 KMP foundation: Gradle modules, build manifests and shared
# fixtures) run the full set; Android behavioral evidence rides at task level
# and phase exit until P12-T05 introduces bounded KMP/iOS check selection.
SELECTION="full"
if [[ -z "$CHANGED_ALL" ]]; then
    SELECTION="full"
else
    DOCS_ONLY=1
    while IFS= read -r f || [[ -n "$f" ]]; do
        case "$f" in
            *.md|*.mdc|docs/*|.cursor/*|README*|ROADMAP*|TRADEMARKS*|LICENSE*|.gitignore)
                ;;
            backend/*|contracts/*|infra/*|scripts/*|.github/*|Makefile|backend/go.mod|backend/go.sum|domain/*|application/*|data/*|app/*|gradle/*|shared/*|iosApp/*|settings.gradle.kts|build.gradle.kts|gradle.properties)
                DOCS_ONLY=0
                ;;
            *)
                echo "ERROR: unclassified changed path: $f" >&2
                echo "Add an explicit selection rule for this area; refusing to guess." >&2
                exit 1
                ;;
        esac
    done <<< "$CHANGED_ALL"
    if [[ "$DOCS_ONLY" == "1" ]]; then
        SELECTION="docs-only"
    fi
fi
echo "== selection: $SELECTION =="

echo "== whitespace =="
git diff --check
# Untracked docs are not in the index; check them explicitly.
if git ls-files --others --exclude-standard -- '*.md' '*.mdc' | grep -q .; then
    git diff --no-index /dev/null /dev/null >/dev/null 2>&1 || true
fi

echo "== secret scan (tracked) =="
bash scripts/scan-secrets.sh

echo "== secret scan (working tree + PR diff) =="
# Scanner scripts carry detection literals; never scan themselves or docs
# prose the same way the tracked scanner excludes them. This step covers the
# committed diff so a clean CI checkout still detects a merged secret.
SECRET_PATTERN='ghp_[A-Za-z0-9]{20,}|github_pat_|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|sk_live_|AKIA[0-9A-Z]{16}|xox[bap]-'
SCAN_LIST="$(printf "%s" "$CHANGED_ALL" | grep -v -e '^scripts/check-backend-fast.sh$' -e '^scripts/scan-secrets.sh$' -e '^scripts/quick-verify.sh$' -e '^scripts/check-security.sh$' -e '^scripts/verify-release.sh$' -e '^scripts/wiki.sh$' -e '^scripts/tests/test-gate-selection.sh$' || true)"
if [[ -n "$SCAN_LIST" ]]; then
    FILES=""
    # shellcheck disable=SC2086
    for f in $SCAN_LIST; do
        if [[ -f "$f" ]]; then
            FILES="$FILES $f"
        fi
    done
    if [[ -n "$FILES" ]]; then
        # shellcheck disable=SC2086
        set +e
        SCAN_OUT="$(grep -nE "$SECRET_PATTERN" $FILES 2>&1)"
        SCAN_EXIT=$?
        set -e
        if [[ "$SCAN_EXIT" -eq 0 ]]; then
            echo "$SCAN_OUT"
            echo "ERROR: potential secret pattern in changed/PR files" >&2
            exit 1
        elif [[ "$SCAN_EXIT" -ne 1 ]]; then
            echo "$SCAN_OUT" >&2
            echo "ERROR: PR-diff secret scanner failed (exit $SCAN_EXIT)" >&2
            exit 1
        fi
    fi
fi

if [[ "$SELECTION" == "docs-only" ]]; then
    echo "== docs-only subset: contract reference integrity =="
    (cd backend && go test -count=1 ./internal/platform/apicontract/...)
    vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
    echo "== docs refs =="
    if ! grep -q "P01-T13" ROADMAP.md; then
        echo "ERROR: ROADMAP missing P01-T13 task marker" >&2
        exit 1
    fi
else
    echo "== full quick: existing fast gate =="
    bash scripts/check-backend-fast.sh
    echo "== vulnerability scan =="
    (cd backend && govulncheck ./...)
    # Bounded KMP/iOS selection (P12-T05): mobile-area changes additionally
    # run the hermetic mobile static guards (no JDK/Xcode needed). Full
    # Gradle/Swift suites stay at task level and phase exit; Quick
    # verification itself is never disabled or skipped by this selection.
    MOBILE_CHANGED=0
    while IFS= read -r f || [[ -n "$f" ]]; do
        case "$f" in
            domain/*|application/*|data/*|app/*|iosApp/*|shared/*|gradle/*|settings.gradle.kts|build.gradle.kts|gradle.properties)
                MOBILE_CHANGED=1
                break
                ;;
        esac
    done <<< "$CHANGED_ALL"
    if [[ "$MOBILE_CHANGED" == "1" ]]; then
        echo "== mobile static selection =="
        bash scripts/check-mobile.sh --static-only
    fi
fi

END_EPOCH="$(date +%s)"
DURATION=$((END_EPOCH - START_EPOCH))
echo "quick-verify duration: ${DURATION}s (budget 300s warm cache; measured, not enforced)"
if [[ "$DURATION" -gt 300 ]]; then
    echo "WARNING: quick-verify exceeded 300s budget; investigate selection or cache" >&2
fi

HEAD_SHA="$(git rev-parse HEAD)"
echo "quick-verify: ok (selection=$SELECTION head=$HEAD_SHA)"
