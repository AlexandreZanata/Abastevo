#!/usr/bin/env bash
# Phase R2.3 / Gate 15 — publish v2.0.0 GitHub Release with APK/AAB assets.
# Default: dry-run (validates only). Pass --publish to push tag and create the release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="v2.0.0"
REPO="AlexandreZanata/TABELA-ANP-COMBUSTIVEIS"
RELEASE_NOTES="$ROOT/docs/releases/v2.0.0.md"
APK_DIR="$ROOT/app/build/outputs/apk/release"
AAB_DIR="$ROOT/app/build/outputs/bundle/release"
PUBLISH=false

for arg in "$@"; do
    case "$arg" in
        --publish) PUBLISH=true ;;
        -h|--help)
            echo "Usage: $0 [--publish]"
            echo "  (default)  Validate Gate 15 publish readiness (dry-run)"
            echo "  --publish  Push main + tag and create GitHub Release with assets"
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $arg"
            exit 1
            ;;
    esac
done

bash "$ROOT/scripts/validate-gate-15-release.sh"

APK="$(find "$APK_DIR" -maxdepth 1 -name '*.apk' -type f | head -n 1)"
AAB="$(find "$AAB_DIR" -maxdepth 1 -name '*.aab' -type f | head -n 1)"

RELEASE_TITLE="$(awk -v start='## Release title' '
    $0 ~ start { capture = 1; next }
    capture && /^```$/ { if (!seen) { seen = 1; next }; capture = 0; next }
    capture && seen { print; exit }
' "$RELEASE_NOTES" | sed '/^[[:space:]]*$/d')"

if [[ -z "$RELEASE_TITLE" ]]; then
    RELEASE_TITLE="ANP Fuel Prices v2.0.0 — Week picker & national search"
fi

NOTES_FILE="$(mktemp)"
trap 'rm -f "$NOTES_FILE"' EXIT
awk -v start='begin:release-body' -v end='end:release-body' '
    index($0, start) { capture = 1; next }
    index($0, end) { capture = 0; next }
    capture { print }
' "$RELEASE_NOTES" > "$NOTES_FILE"

if [[ ! -s "$NOTES_FILE" ]]; then
    echo "ERROR: release body is empty in $RELEASE_NOTES"
    exit 1
fi

echo "OK: release title — $RELEASE_TITLE"
echo "OK: release body — $(wc -l < "$NOTES_FILE") lines"
echo "OK: APK asset — $APK"
echo "OK: AAB asset — $AAB"

if [[ "$PUBLISH" != true ]]; then
    echo ""
    echo "DRY RUN — publish readiness validated (R2.3.2 + R2.3.3)."
    echo "Maintainer steps (R2.3.1):"
    echo "  git push origin main && git push origin $TAG"
    echo ""
    echo "Then create GitHub Release:"
    echo "  gh release create $TAG \\"
    echo "    --repo $REPO \\"
    echo "    --title \"$RELEASE_TITLE\" \\"
    echo "    --notes-file $NOTES_FILE \\"
    echo "    \"$APK\" \"$AAB\""
    exit 0
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "ERROR: working tree is not clean — commit before publishing"
    exit 1
fi

LOCAL_TAG_COMMIT="$(git rev-parse "$TAG^{commit}")"
if git ls-remote --exit-code origin "refs/tags/$TAG" >/dev/null 2>&1; then
    REMOTE_TAG_COMMIT="$(git ls-remote origin "refs/tags/$TAG" | awk '{print $1}')"
    if [[ "$REMOTE_TAG_COMMIT" != "$(git rev-parse "$LOCAL_TAG_COMMIT")" ]]; then
        echo "ERROR: remote tag $TAG points to a different commit"
        exit 1
    fi
    echo "OK: remote tag $TAG already exists"
else
    echo "Pushing main and tag $TAG..."
    git push origin main
    git push origin "$TAG"
fi

if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
    echo "OK: GitHub Release $TAG already exists"
    exit 0
fi

echo "Creating GitHub Release $TAG..."
gh release create "$TAG" \
    --repo "$REPO" \
    --title "$RELEASE_TITLE" \
    --notes-file "$NOTES_FILE" \
    "$APK" "$AAB"

echo "GitHub Release $TAG published with APK and AAB assets."
