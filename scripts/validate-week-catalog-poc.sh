#!/usr/bin/env bash
# Phase 12.2.5 — live ANP listing week catalog validation.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

POC_DIR="$ROOT/.local/poc-results"
LIVE_HTML="$POC_DIR/anp-listing-live-snapshot.html"
POC_MD="$POC_DIR/week-catalog-poc.md"
LISTING_URL="https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas"
USER_AGENT="AnpFuel/1.0 (Android; open-source fuel price reader)"

mkdir -p "$POC_DIR"

echo "Fetching live ANP listing page..."
http_code="$(curl -sSL -A "$USER_AGENT" -o "$LIVE_HTML" -w '%{http_code}' "$LISTING_URL")"
if [[ "$http_code" != "200" ]]; then
    echo "ERROR: listing page returned HTTP $http_code"
    exit 1
fi
echo "OK: listing page HTTP 200 ($(wc -c < "$LIVE_HTML") bytes)"

echo "Running live catalog validation..."
ANP_LIVE_POC=true \
ANP_LIVE_HTML_PATH="$LIVE_HTML" \
ANP_LIVE_POC_OUTPUT="$POC_MD" \
    ./gradlew :data:testDebugUnitTest \
        --tests "com.anpfuel.data.remote.AnpListingLiveCatalogValidationTest" \
        --no-daemon -q

if [[ ! -f "$POC_MD" ]]; then
    echo "ERROR: POC markdown not written to $POC_MD"
    exit 1
fi

echo "OK: wrote $POC_MD"
echo "Week catalog live POC validation passed."
