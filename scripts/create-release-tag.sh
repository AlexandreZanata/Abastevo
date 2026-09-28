#!/usr/bin/env bash
# Phase 15 / Gate 15 — runs release checks and creates annotated tag v2.0.0 locally.
# Does not push to remote; maintainer publishes GitHub Release separately.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="v2.0.0"
RELEASE_NOTES="$ROOT/docs/releases/v2.0.0.md"

bash "$ROOT/scripts/validate-release-tag.sh"
bash "$ROOT/scripts/validate-release-build.sh"

TAG_MESSAGE="$(awk -v start='## Release title' '
    $0 ~ start { capture = 1; next }
    capture && /^```$/ { if (!seen) { seen = 1; next }; capture = 0; next }
    capture && seen { print; exit }
' "$RELEASE_NOTES" | sed '/^[[:space:]]*$/d')"

if [[ -z "$TAG_MESSAGE" ]]; then
    TAG_MESSAGE="ANP Fuel Prices v2.0.0 — Week picker & national search"
fi

if git rev-parse "$TAG" >/dev/null 2>&1; then
    TAG_COMMIT="$(git rev-parse "$TAG^{commit}")"
    HEAD_COMMIT="$(git rev-parse HEAD)"
    if [[ "$TAG_COMMIT" == "$HEAD_COMMIT" ]]; then
        echo "OK: tag $TAG already exists on HEAD ($HEAD_COMMIT)"
        exit 0
    fi
    echo "ERROR: tag $TAG exists but points to $TAG_COMMIT, not HEAD $HEAD_COMMIT"
    exit 1
fi

git tag -a "$TAG" -m "$(cat <<EOF
$TAG_MESSAGE

v2 release: week picker, national search, safe areas, fuel icons.
Fuel price data © ANP. This app is not affiliated with ANP.

Refs: Phase 15, Gate 15, BR-009
EOF
)"

echo "Created annotated tag $TAG at $(git rev-parse --short HEAD)"
echo "Verify: git show $TAG"
echo "Publish when ready: git push origin main && git push origin $TAG"
