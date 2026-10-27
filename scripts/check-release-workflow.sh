#!/usr/bin/env bash
# Guards the public staging release workflow against supply-chain and secret
# exposure regressions: unpinned actions, pull_request_target, non-token
# secrets, shell tracing, environment dumps, echo of variables and latest tags.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOW="${1:-$ROOT/.github/workflows/release-staging.yml}"

if [[ ! -f "$WORKFLOW" ]]; then
    echo "ERROR: release workflow not found: $WORKFLOW" >&2
    exit 1
fi

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

if grep -nE 'pull_request_target' "$WORKFLOW" >/dev/null; then
    fail "pull_request_target is forbidden"
fi

if grep -nE '^[[:space:]]{2}pull_request:' "$WORKFLOW" >/dev/null; then
    fail "pull_request trigger is forbidden for the release workflow"
fi

if grep -nE 'permissions:[[:space:]]*write-all' "$WORKFLOW" >/dev/null; then
    fail "write-all permissions are forbidden"
fi

if grep -nE ':[[:space:]]*latest([[:space:]]|$)' "$WORKFLOW" >/dev/null; then
    fail "latest tag is forbidden"
fi

if grep -nE '(^|[[:space:]])set[[:space:]]+-[A-Za-z]*x' "$WORKFLOW" >/dev/null; then
    fail "set -x is forbidden"
fi

if grep -nE '\becho\b.*\$' "$WORKFLOW" >/dev/null; then
    fail "echo of variables is forbidden"
fi

if grep -nE 'printenv' "$WORKFLOW" >/dev/null; then
    fail "environment dump is forbidden"
fi

if grep -nE '(^|[[:space:];&|])env([[:space:]]|$|[>|])' "$WORKFLOW" >/dev/null; then
    fail "environment dump is forbidden"
fi

if grep -nE 'export[[:space:]]+-p|declare[[:space:]]+-x' "$WORKFLOW" >/dev/null; then
    fail "environment dump is forbidden"
fi

while IFS= read -r secret; do
    if [[ "$secret" != "secrets.GITHUB_TOKEN" ]]; then
        fail "secret reference is forbidden: $secret"
    fi
done < <(grep -oE 'secrets\.[A-Za-z0-9_]+' "$WORKFLOW" || true)

while IFS= read -r line; do
    ref="${line#*uses:}"
    ref="${ref%%#*}"
    ref="$(printf '%s' "$ref" | tr -d '[:space:]')"
    case "$ref" in
        ./*) continue ;;
    esac
    if [[ ! "$ref" =~ ^[^@]+@[0-9a-f]{40}$ ]]; then
        fail "action is not pinned by a full commit SHA: $ref"
    fi
done < <(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:' "$WORKFLOW" || true)

echo "Release workflow guard passed."
