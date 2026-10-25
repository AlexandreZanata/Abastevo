#!/usr/bin/env bash
# ESP-18 post-publication public smoke (read-only). It requires an approved
# public HTTPS origin and the exact expected portal content hash. It is never
# run automatically and performs no mutation, DNS/CDN change or publication.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
PREFLIGHT="$ROOT/infra/scripts/load/publication_preflight.py"

log() { echo "public-smoke: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }
fail() { echo "FAIL: $*" >&2; exit 1; }

if [[ "${1:-}" == "--self-test" ]]; then
    python3 "$PREFLIGHT" --check-origin https://portal.abastevo.com.br >/dev/null || fail "public origin rejected"
    for origin in http://portal.abastevo.com.br https://localhost https://portal.example https://10.0.0.1; do
        if python3 "$PREFLIGHT" --check-origin "$origin" >/dev/null 2>&1; then
            fail "unsafe origin accepted: $origin"
        fi
    done
    echo "public smoke self-test ok"
    exit 0
fi

ORIGIN=""; EXPECTED=""; PAGES="/consultar/,/fiscalizacao/,/metodologia/,/dados/"; OUT=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --origin) ORIGIN="${2:-}"; shift 2 ;;
        --expected-sha) EXPECTED="${2:-}"; shift 2 ;;
        --pages) PAGES="${2:-}"; shift 2 ;;
        --out) OUT="${2:-}"; shift 2 ;;
        *) refuse "unknown flag $1" ;;
    esac
done
[[ -n "$ORIGIN" && -n "$EXPECTED" ]] || refuse "--origin and --expected-sha are required"
python3 "$PREFLIGHT" --check-origin "$ORIGIN" >/dev/null || refuse "origin is not an approved public HTTPS origin"
[[ "$EXPECTED" =~ ^sha256:[0-9a-f]{64}$ ]] || refuse "expected hash must be sha256:<64 hex>"
command -v curl >/dev/null || refuse "curl required"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
if [[ -n "$OUT" ]]; then
    mkdir -p "$(dirname "$OUT")"
fi
log "smoke target $ORIGIN expected=$EXPECTED"

curl -fsS --max-time 15 "$ORIGIN/portal-manifest.json" -o "$TMP/portal-manifest.json" || fail "portal manifest unavailable"
CONTENT="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("content_sha256",""))' "$TMP/portal-manifest.json")"
[[ "$CONTENT" == "$EXPECTED" ]] || fail "portal manifest content hash $CONTENT != $EXPECTED"

IFS=',' read -r -a page_list <<<"$PAGES"
[[ "${#page_list[@]}" -gt 0 ]] || fail "no pages configured"
FIRST_PAGE="$(echo "${page_list[0]}" | xargs)"
[[ -n "$FIRST_PAGE" ]] || fail "empty first page"
curl -fsS --max-time 15 -D "$TMP/first.headers" -o "$TMP/first.html" -H 'Accept-Encoding: gzip' "$ORIGIN$FIRST_PAGE" || fail "first page unavailable"
grep -qi '^strict-transport-security:' "$TMP/first.headers" || fail "HSTS missing"
grep -qi '^x-content-type-options: nosniff' "$TMP/first.headers" || fail "nosniff missing"
grep -qi '^cache-control: public, max-age=0, must-revalidate' "$TMP/first.headers" || fail "document revalidation missing"
grep -qi '^content-encoding: gzip' "$TMP/first.headers" || fail "HTML gzip missing"
if grep -qi '^x-robots-tag:.*noindex' "$TMP/first.headers"; then
    fail "public page still sends X-Robots-Tag noindex"
fi

curl -fsS --max-time 15 "$ORIGIN/robots.txt" -o "$TMP/robots.txt" || fail "robots.txt unavailable"
grep -q 'Allow: /' "$TMP/robots.txt" || fail "robots.txt does not allow indexing"
grep -q "Sitemap: $ORIGIN/sitemap.xml" "$TMP/robots.txt" || fail "robots.txt sitemap mismatch"
curl -fsS --max-time 15 "$ORIGIN/sitemap.xml" -o "$TMP/sitemap.xml" || fail "sitemap unavailable"
python3 "$PREFLIGHT" --check-sitemap-origin "$ORIGIN" "$TMP/sitemap.xml" >/dev/null || fail "sitemap origin mismatch"

for page in "${page_list[@]}"; do
    page="$(echo "$page" | xargs)"
    [[ -n "$page" ]] || continue
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$ORIGIN$page")"
    [[ "$code" == "200" ]] || fail "page $page returned $code"
done
[[ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$ORIGIN/nao-existe/")" == "404" ]] || fail "custom 404 missing"
[[ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$ORIGIN/v1/health/live")" == "404" ]] || fail "API path served"
[[ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$ORIGIN/health/live")" == "404" ]] || fail "health path served"

cat > "$TMP/summary.txt" <<SUMMARY
public smoke PASS
origin=$ORIGIN
content=$EXPECTED
pages=${PAGES}
indexable=true
api_proxy=false
SUMMARY
cat "$TMP/summary.txt"
if [[ -n "$OUT" ]]; then
    cp "$TMP/summary.txt" "$OUT"
fi
