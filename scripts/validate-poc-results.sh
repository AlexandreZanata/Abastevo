#!/usr/bin/env bash
# Appendix B — validates POC results documentation in .local/poc-results/.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
POC_DIR="$ROOT/.local/poc-results"

REQUIRED_FILES=(
    "parser-poc.md"
    "database-poc.md"
    "network-poc.md"
)

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

if [[ ! -d "$POC_DIR" ]]; then
    echo "ERROR: missing POC results directory at $POC_DIR"
    exit 1
fi
echo "OK: .local/poc-results/ directory present"

for file in "${REQUIRED_FILES[@]}"; do
    assert_file "$POC_DIR/$file" "$file"
done

PARSER_TEXT="$(cat "$POC_DIR/parser-poc.md")"
DATABASE_TEXT="$(cat "$POC_DIR/database-poc.md")"
NETWORK_TEXT="$(cat "$POC_DIR/network-poc.md")"

assert_contains "parser_gate" "$PARSER_TEXT" "Gate 2"
assert_contains "parser_rows" "$PARSER_TEXT" "19,676"

assert_contains "database_gate" "$DATABASE_TEXT" "Gate 3"
assert_contains "database_fts" "$DATABASE_TEXT" "100 ms"
assert_contains "database_br003" "$DATABASE_TEXT" "BR-003"
assert_contains "database_size" "$DATABASE_TEXT" "15 MB"

assert_contains "network_gate" "$NETWORK_TEXT" "Gate 4"
assert_contains "network_live" "$NETWORK_TEXT" "2026-06-18"

echo "POC results validation passed (Appendix B)."
