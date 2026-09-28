#!/usr/bin/env bash
# Phase 9.4.2 — scans tracked files for accidental secret commits.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TRACKED_SECRET_FILES=(
    "local.properties"
    "secrets.properties"
    "signing.properties"
    ".env"
    "google-services.json"
)

for file in "${TRACKED_SECRET_FILES[@]}"; do
    if git ls-files --error-unmatch "$file" >/dev/null 2>&1; then
        echo "ERROR: tracked secret file detected: $file"
        exit 1
    fi
done

PATTERNS=(
    'ghp_[A-Za-z0-9]{20,}'
    'github_pat_'
    'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY'
    'sk_live_'
    'AKIA[0-9A-Z]{16}'
)

PATH_SPECS=(
    ':!*.md'
    ':!scripts/scan-secrets.sh'
    ':!docs/**'
    ':!.cursor/**'
    ':!data/src/main/kotlin/com/anpfuel/data/local/fts/**'
    ':!data/src/main/kotlin/com/anpfuel/data/local/entity/MunicipalityFtsEntity.kt'
    ':!data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabaseMigrations.kt'
    ':!data/src/test/**'
)

for pattern in "${PATTERNS[@]}"; do
    if git grep -nE "$pattern" -- "${PATH_SPECS[@]}" >/dev/null 2>&1; then
        echo "ERROR: potential secret pattern matched: $pattern"
        git grep -nE "$pattern" -- "${PATH_SPECS[@]}"
        exit 1
    fi
done

echo "Secret scan passed — no tracked secrets or high-confidence patterns found."
