#!/usr/bin/env bash
# Phase 0.1 — validates repository baseline: gitignore, cursor rules, commit conventions (Gate 0.1).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GITIGNORE="$ROOT/.gitignore"
COMMIT_CONVENTIONS="$ROOT/docs/commit-conventions.md"
CURSOR_RULES="$ROOT/.cursor/rules"

assert_gitignore_contains() {
    local pattern="$1"
    local label="$2"
    if ! grep -Fq "$pattern" "$GITIGNORE"; then
        echo "ERROR: .gitignore missing $label (expected: $pattern)"
        exit 1
    fi
    echo "OK: .gitignore covers $label"
}

assert_file() {
    local path="$1"
    local label="$2"
    if [[ ! -f "$path" ]]; then
        echo "ERROR: missing $label at $path"
        exit 1
    fi
    echo "OK: $label present"
}

assert_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "ERROR: $label missing required text: $needle"
        exit 1
    fi
    echo "OK: $label contains '$needle'"
}

# 0.1.1 — .gitignore covers Gradle, IDE, secrets, data/downloads/
assert_file "$GITIGNORE" ".gitignore"

assert_gitignore_contains ".gradle/" "Gradle cache"
assert_gitignore_contains "build/" "Gradle build output"
assert_gitignore_contains "local.properties" "local.properties"
assert_gitignore_contains ".idea/" "IDE (.idea)"
assert_gitignore_contains "*.iml" "IDE (*.iml)"
assert_gitignore_contains "*.keystore" "keystores"
assert_gitignore_contains "signing.properties" "signing.properties"
assert_gitignore_contains "data/downloads/" "data/downloads"

# 0.1.2 — .cursor/rules/ present (agent core + stack + i18n + commits)
if [[ ! -d "$CURSOR_RULES" ]]; then
    echo "ERROR: missing .cursor/rules/ directory"
    exit 1
fi
echo "OK: .cursor/rules/ directory present"

REQUIRED_RULES=(
    "agent-core-principles.mdc"
    "agent-core-practices.mdc"
    "project-stack-architecture.mdc"
    "project-internationalization.mdc"
    "commit-conventions.mdc"
)

for rule in "${REQUIRED_RULES[@]}"; do
    assert_file "$CURSOR_RULES/$rule" "cursor rule $rule"
done

# 0.1.3 — commit conventions documented (Conventional Commits in English)
assert_file "$COMMIT_CONVENTIONS" "docs/commit-conventions.md"

CONVENTIONS_TEXT="$(cat "$COMMIT_CONVENTIONS")"
assert_contains "conventional_commits" "$CONVENTIONS_TEXT" "Conventional Commits"
assert_contains "english_commits" "$CONVENTIONS_TEXT" "must be in English"
assert_contains "commit_format" "$CONVENTIONS_TEXT" "<type>(<scope>): <subject>"

# Gate 0.1 — no secrets tracked
bash "$ROOT/scripts/scan-secrets.sh"

echo "Repository baseline validation passed (Gate 0.1)."
