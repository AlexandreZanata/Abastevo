#!/usr/bin/env bash
# ESP-17 isolated HTTPS preview/accessibility rehearsal. It uses Caddy's local
# internal CA, a private container, a noindex edge and a dedicated preview
# release root. It never touches the shared edge, DNS, API, database or ANP.
#
# Required:
#   PORTAL_PREVIEW_ROOT          new private release root
#   PORTAL_PREVIEW_ARTIFACT_A    noindex verified candidate
#   PORTAL_PREVIEW_ARTIFACT_B    full/corrected verified candidate
#   PORTAL_SITE_URL              origin bound in the artifact manifest
# Optional:
#   PORTAL_RESULTS_DIR, PORTAL_HTTPS_ORIGIN, PORTAL_HTTP_ORIGIN,
#   PORTAL_STATION_ID, PORTAL_CHROME
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PREVIEW_ROOT="${PORTAL_PREVIEW_ROOT:?PORTAL_PREVIEW_ROOT is required}"
ARTIFACT_A="${PORTAL_PREVIEW_ARTIFACT_A:?PORTAL_PREVIEW_ARTIFACT_A is required}"
ARTIFACT_B="${PORTAL_PREVIEW_ARTIFACT_B:?PORTAL_PREVIEW_ARTIFACT_B is required}"
SITE_URL="${PORTAL_SITE_URL:?PORTAL_SITE_URL is required}"
HTTPS="${PORTAL_HTTPS_ORIGIN:-https://localhost:18443}"
HTTP="${PORTAL_HTTP_ORIGIN:-http://localhost:18082}"
STATION_ID="${PORTAL_STATION_ID:-00000000-0000-4000-8000-000000000001}"
RESULTS="${PORTAL_RESULTS_DIR:-/tmp/abastevo-portal-preview-$(date -u +%Y%m%dT%H%M%SZ)}"
CADDY_IMAGE='caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d'
CADDYFILE="$ROOT/infra/caddy/portal-preview.Caddyfile"
CHROME="${PORTAL_CHROME:-google-chrome}"
WORK="$(mktemp -d /tmp/abastevo-portal-preview-work.XXXXXX)"
NAME="abastevo-portal-preview-$$"
NET="abastevo-portal-preview-$$-$(date +%s)"
CAS=""

log() { echo "preview-portal: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }
cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
    rm -rf "$WORK"
}
trap cleanup EXIT

[[ ! -e "$PREVIEW_ROOT" ]] || refuse "preview root already exists: $PREVIEW_ROOT"
mkdir -p "$RESULTS"
chmod 700 "$RESULTS"
[[ -f "$CADDYFILE" ]] || refuse "preview Caddyfile missing"
[[ -f "$ARTIFACT_A/portal-manifest.json" && -f "$ARTIFACT_B/portal-manifest.json" ]] || refuse "preview artifacts missing"
docker image inspect "$CADDY_IMAGE" >/dev/null 2>&1 || refuse "pinned Caddy image unavailable"
command -v curl >/dev/null && command -v openssl >/dev/null && command -v node >/dev/null && command -v "$CHROME" >/dev/null || refuse "required client tool missing"

(cd backend && go build -o "$WORK/portalrelease" ./cmd/portalrelease)

log "promoting noindex preview release A"
"$WORK/portalrelease" promote --artifact "$ARTIFACT_A" --root "$PREVIEW_ROOT" --site-url "$SITE_URL" >/dev/null
"$WORK/portalrelease" verify --root "$PREVIEW_ROOT" --site-url "$SITE_URL" | tee "$RESULTS/release-a.txt" >/dev/null
A_HASH="$(sed -n 's/.*current=\([^ ]*\).*/\1/p' "$RESULTS/release-a.txt")"
[[ -n "$A_HASH" ]] || refuse "preview release A did not become current"

log "starting HTTPS preview on $HTTPS"
docker network create "$NET" >/dev/null
docker run --rm -d --name "$NAME" --network "$NET" \
    -p "127.0.0.1:18443:18443" -p "127.0.0.1:18082:18082" \
    -v "$CADDYFILE:/etc/caddy/Caddyfile:ro" \
    -v "$PREVIEW_ROOT:/srv/portal:ro" \
    "$CADDY_IMAGE" >/dev/null

CA="$RESULTS/caddy-root.crt"
for _ in $(seq 1 100); do
    if docker cp "$NAME:/data/caddy/pki/authorities/local/root.crt" "$CA" >/dev/null 2>&1; then
        break
    fi
    sleep 0.1
done
[[ -s "$CA" ]] || { docker logs "$NAME" >&2 || true; refuse "Caddy internal CA unavailable"; }
chmod 600 "$CA"

READY=0
for _ in $(seq 1 100); do
    if [[ "$(curl --cacert "$CA" -sS -o /dev/null -w '%{http_code}' --max-time 2 "$HTTPS/consultar/" 2>/dev/null || true)" == "200" ]]; then
        READY=1
        break
    fi
    sleep 0.1
done
[[ "$READY" == "1" ]] || { docker logs "$NAME" >&2 || true; refuse "HTTPS preview did not become ready"; }
log "HTTPS preview ready"

curl --cacert "$CA" -sS -D "$RESULTS/consultar.headers" -o /dev/null "$HTTPS/consultar/"
grep -qi '^strict-transport-security:' "$RESULTS/consultar.headers" || refuse "HSTS header missing"
grep -qi '^x-content-type-options: nosniff' "$RESULTS/consultar.headers" || refuse "nosniff missing"
grep -qi '^x-robots-tag: noindex' "$RESULTS/consultar.headers" || refuse "preview noindex header missing"
grep -qi '^cache-control: public, max-age=0, must-revalidate' "$RESULTS/consultar.headers" || refuse "HTML revalidation header missing"
curl --cacert "$CA" -sS -D "$RESULTS/assets.headers" -o /dev/null -H 'Accept-Encoding: gzip' "$HTTPS/$(curl --cacert "$CA" -sS "$HTTPS/consultar/" | sed -n 's/.*href="\(\/assets\/[^"]*\.css\)".*/\1/p' | head -1 | sed 's#^/##')"
grep -qi '^content-encoding: gzip' "$RESULTS/assets.headers" || refuse "asset gzip missing"
grep -qi '^cache-control: public, max-age=31536000, immutable' "$RESULTS/assets.headers" || refuse "immutable cache header missing"
CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$HTTP/consultar/")"
[[ "$CODE" == "308" ]] || refuse "HTTP->HTTPS redirect got $CODE"
REDIRECT="$(curl -sS -o /dev/null -w '%{redirect_url}' --max-time 5 "$HTTP/consultar/")"
[[ "$REDIRECT" == "$HTTPS/consultar/" ]] || refuse "redirect target=$REDIRECT"
echo | openssl s_client -connect localhost:18443 -servername localhost -CAfile "$CA" -verify_return_error -tls1_2 2>&1 | grep -q 'Verify return code: 0' || refuse "TLS 1.2 verification failed"
CODE="$(curl --cacert "$CA" -sS -o /dev/null -w '%{http_code}' --max-time 5 "$HTTPS/v1/health/live")"
[[ "$CODE" == "404" ]] || refuse "API path served in preview: $CODE"
CODE="$(curl --cacert "$CA" -sS -o /dev/null -w '%{http_code}' --max-time 5 "$HTTPS/sitemap.xml")"
[[ "$CODE" == "404" ]] || refuse "noindex A unexpectedly has a sitemap"
grep -q '<script type="module"' <<<"$(curl --cacert "$CA" -sS "$HTTPS/consultar/")" && refuse "noindex A unexpectedly has a script" || true
log "HTTPS/header/redirect/noindex/no-API checks: PASS"

node infra/scripts/load/preview_probe.mjs --origin "$HTTPS" --insecure --expect-js false \
    --station "$STATION_ID" --screenshots "$RESULTS/screenshots-a" --out "$RESULTS/preview-a.json" \
    2>&1 | tee "$RESULTS/preview-a.log"

log "promoting corrected/full release B"
"$WORK/portalrelease" promote --artifact "$ARTIFACT_B" --root "$PREVIEW_ROOT" --site-url "$SITE_URL" \
    --fence --expect-current "$A_HASH" | tee "$RESULTS/release-b.txt" >/dev/null
B_HASH="$(sed -n 's/.*current=\([^ ]*\).*/\1/p' "$RESULTS/release-b.txt")"
[[ -n "$B_HASH" && "$B_HASH" != "$A_HASH" ]] || refuse "release B did not switch"
CODE="$(curl --cacert "$CA" -sS -o /dev/null -w '%{http_code}' --max-time 5 "$HTTPS/sitemap.xml")"
[[ "$CODE" == "200" ]] || refuse "corrected B sitemap got $CODE"
grep -q '<script type="module"' <<<"$(curl --cacert "$CA" -sS "$HTTPS/consultar/")" || refuse "corrected B script missing"
node infra/scripts/load/preview_probe.mjs --origin "$HTTPS" --insecure --expect-js true \
    --station "$STATION_ID" --screenshots "$RESULTS/screenshots-b" --out "$RESULTS/preview-b.json" \
    2>&1 | tee "$RESULTS/preview-b.log"

log "rolling back to noindex preview A"
"$WORK/portalrelease" rollback --root "$PREVIEW_ROOT" --target "$A_HASH" --site-url "$SITE_URL" \
    | tee "$RESULTS/release-recovered.txt" >/dev/null
CODE="$(curl --cacert "$CA" -sS -o /dev/null -w '%{http_code}' --max-time 5 "$HTTPS/sitemap.xml")"
[[ "$CODE" == "404" ]] || refuse "rollback A sitemap got $CODE"
node infra/scripts/load/preview_probe.mjs --origin "$HTTPS" --insecure --expect-js false \
    --station "$STATION_ID" --out "$RESULTS/preview-recovered.json" \
    2>&1 | tee "$RESULTS/preview-recovered.log"

log "rehearsing release-root backup/restore"
tar -C "$PREVIEW_ROOT" -czf "$RESULTS/portal-release-backup.tar.gz" state.json releases current
sha256sum "$RESULTS/portal-release-backup.tar.gz" > "$RESULTS/portal-release-backup.sha256"
mkdir -p "$RESULTS/restore-root"
tar -C "$RESULTS/restore-root" -xzf "$RESULTS/portal-release-backup.tar.gz"
"$WORK/portalrelease" verify --root "$RESULTS/restore-root" --site-url "$SITE_URL" | tee "$RESULTS/restore-verify.txt" >/dev/null
"$WORK/portalrelease" rollback --root "$RESULTS/restore-root" --target "$B_HASH" --site-url "$SITE_URL" \
    | tee "$RESULTS/restore-rollback-b.txt" >/dev/null
"$WORK/portalrelease" rollback --root "$RESULTS/restore-root" --target "$A_HASH" --site-url "$SITE_URL" \
    | tee "$RESULTS/restore-rollback-a.txt" >/dev/null
log "backup/restore rehearsal: PASS"

cat > "$RESULTS/summary.txt" <<SUMMARY
ESP-17 preview rehearsal PASS
https=$HTTPS
artifact_a=$A_HASH
artifact_b=$B_HASH
preview_current=$A_HASH (after rollback)
screenshots=$RESULTS/screenshots-a,$RESULTS/screenshots-b
backup=$RESULTS/portal-release-backup.tar.gz
scope=isolated local HTTPS preview; internal CA; no shared edge/DNS/API/DB/ANP
SUMMARY
cat "$RESULTS/summary.txt"
