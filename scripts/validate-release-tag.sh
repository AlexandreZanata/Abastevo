#!/usr/bin/env bash
# Phase 15 / Gate 15 — validates repository readiness for v2.0.0 release tag.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="v2.0.0"
GRADLE="$ROOT/app/build.gradle.kts"
LICENSE="$ROOT/LICENSE"
README="$ROOT/README.md"
RELEASE_NOTES="$ROOT/docs/releases/v2.0.0.md"

assert_file() {
    local path="$1"
    local label="$2"
    if [[ ! -f "$path" ]]; then
        echo "ERROR: missing $label at $path"
        exit 1
    fi
    echo "OK: $label present"
}

assert_file "$LICENSE" "LICENSE"
assert_file "$GRADLE" "app/build.gradle.kts"
assert_file "$README" "README.md"
assert_file "$RELEASE_NOTES" "release notes"

README_TEXT="$(cat "$README")"
if [[ "$README_TEXT" != *"MIT License"* ]]; then
    echo "ERROR: README must reference MIT License (Gate 15)"
    exit 1
fi
echo "OK: README references MIT License"

if [[ "$README_TEXT" != *"docs/releases/v2.0.0.md"* ]]; then
    echo "ERROR: README must link to v2.0.0 release notes (Gate 15)"
    exit 1
fi
echo "OK: README links to v2.0.0 release notes"

VERSION_NAME="$(grep 'versionName' "$GRADLE" | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/')"
VERSION_CODE="$(grep 'versionCode' "$GRADLE" | head -n 1 | sed -E 's/.*= ([0-9]+).*/\1/')"

if [[ "$VERSION_NAME" != "2.0.0" ]]; then
    echo "ERROR: expected versionName 2.0.0, found '$VERSION_NAME'"
    exit 1
fi
echo "OK: versionName is 2.0.0"

if [[ -z "$VERSION_CODE" || "$VERSION_CODE" -lt 2 ]]; then
    echo "ERROR: invalid versionCode '$VERSION_CODE' (expected >= 2 for v2.0.0)"
    exit 1
fi
echo "OK: versionCode is $VERSION_CODE"

bash "$ROOT/scripts/validate-github-release-notes.sh"
bash "$ROOT/scripts/validate-play-store-listing.sh"

if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: working tree is not clean — commit changes before tagging"
    git status --short
    exit 1
fi
echo "OK: working tree clean"

if git rev-parse "$TAG" >/dev/null 2>&1; then
    TAG_COMMIT="$(git rev-parse "$TAG^{commit}")"
    HEAD_COMMIT="$(git rev-parse HEAD)"
    if [[ "$TAG_COMMIT" == "$HEAD_COMMIT" ]]; then
        echo "OK: tag $TAG already exists on HEAD ($HEAD_COMMIT)"
    else
        echo "ERROR: tag $TAG exists but points to $TAG_COMMIT, not HEAD $HEAD_COMMIT"
        exit 1
    fi
else
    echo "OK: tag $TAG not yet created"
fi

echo "Release tag readiness validation passed."
