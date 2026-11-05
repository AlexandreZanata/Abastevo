#!/usr/bin/env bash
# Manual retry entry point for the same guarded main-only Android workflow.
# This command never pushes a branch/tag or uploads an APK directly.
set -euo pipefail
PUBLISH=false
for arg in "$@"; do
    case "$arg" in
        --publish) PUBLISH=true ;;
        -h|--help)
            printf '%s\n' 'Usage: scripts/publish-github-release.sh [--publish]' \
              'Default: show the current main SHA. --publish queues the guarded Android workflow.'
            exit 0 ;;
        *) printf 'Unknown argument: %s\n' "$arg" >&2; exit 1 ;;
    esac
done
REPO='AlexandreZanata/Abastevo'
TARGET_SHA="$(gh api "repos/$REPO/git/ref/heads/main" --jq .object.sha)"
[[ "$TARGET_SHA" =~ ^[a-f0-9]{40}$ ]] || { printf '%s\n' 'Invalid main SHA' >&2; exit 1; }
if [[ "$PUBLISH" == true ]]; then
    gh workflow run android-release.yml --repo "$REPO" --ref main -f "sha=$TARGET_SHA"
    printf '%s\n' 'Queued Android release. Signing/publication still require all three current-main checks.'
else
    printf 'Android release target: %s\n' "$TARGET_SHA"
    printf '%s\n' 'Dry run; use --publish to queue android-release.yml with its existing gates.'
fi
