#!/usr/bin/env bash
# Manual retry/dispatch entry point for the guarded main-only Google Play deploy workflow.
# Never pushes a branch/tag or uploads an AAB directly from a developer machine.
set -euo pipefail

PUBLISH=false
TRACK="production"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --publish)
            PUBLISH=true
            shift
            ;;
        --track)
            [[ $# -ge 2 ]] || { printf '%s\n' '--track requires a value' >&2; exit 1; }
            TRACK="$2"
            shift 2
            ;;
        -h|--help)
            printf '%s\n' 'Usage: scripts/publish-play-release.sh [--publish] [--track production|internal|alpha|beta]' \
              'Default: show the current main SHA. --publish queues android-play-deploy.yml.'
            exit 0
            ;;
        *)
            printf 'Unknown argument: %s\n' "$1" >&2
            exit 1
            ;;
    esac
done

case "$TRACK" in
    production|internal|alpha|beta) ;;
    *)
        printf 'Invalid track: %s (expected production, internal, alpha or beta)\n' "$TRACK" >&2
        exit 1
        ;;
esac

REPO='AlexandreZanata/Abastevo'
TARGET_SHA="$(gh api "repos/$REPO/git/ref/heads/main" --jq .object.sha)"
[[ "$TARGET_SHA" =~ ^[a-f0-9]{40}$ ]] || { printf '%s\n' 'Invalid main SHA' >&2; exit 1; }

if [[ "$PUBLISH" == true ]]; then
    gh workflow run android-play-deploy.yml --repo "$REPO" --ref main -f "sha=$TARGET_SHA" -f "track=$TRACK"
    printf 'Queued Google Play deploy (%s). Signing/upload still require all three current-main checks.\n' "$TRACK"
else
    printf 'Google Play deploy target: %s (track: %s)\n' "$TARGET_SHA" "$TRACK"
    printf '%s\n' 'Dry run; use --publish to queue android-play-deploy.yml with its existing gates.'
fi
