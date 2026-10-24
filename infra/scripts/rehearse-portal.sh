#!/usr/bin/env bash
# ESP-14 isolated static-portal rehearsal. No shared edge, API, DNS or
# production root is touched. It promotes verified candidates into a private
# release root and serves the current symlink with the pinned Caddy image on
# loopback only.
#
# Required:
#   PORTAL_ARTIFACT_A   verified portal candidate
# Optional:
#   PORTAL_ARTIFACT_B   second verified candidate for switch/rollback tests
#   PORTAL_RELEASE_ROOT private new directory (default: mktemp)
#   PORTAL_SITE_URL     origin bound in the artifact manifest (default:
#                       https://portal.example)
#   PORTAL_HTTP_PORT    loopback port (default: 18080)
#   PORTAL_KEEP=1       keep the release root after the rehearsal
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ARTIFACT_A="${PORTAL_ARTIFACT_A:?PORTAL_ARTIFACT_A is required}"
ARTIFACT_B="${PORTAL_ARTIFACT_B:-}"
SITE_URL="${PORTAL_SITE_URL:-https://portal.example}"
HTTP_PORT="${PORTAL_HTTP_PORT:-18080}"
ORIGIN="http://127.0.0.1:${HTTP_PORT}"
CADDY_IMAGE='caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d'
CADDYFILE="$ROOT/infra/caddy/portal-rehearsal.Caddyfile"
KEEP="${PORTAL_KEEP:-0}"

WORK="$(mktemp -d /tmp/abastevo-portal-rehearsal.XXXXXX)"
BIN="$WORK/portalrelease"
NAME="abastevo-portal-rehearsal-$$"
ROOT_TEMP=0
if [[ -n "${PORTAL_RELEASE_ROOT:-}" ]]; then
    RELEASE_ROOT="$PORTAL_RELEASE_ROOT"
    [[ ! -e "$RELEASE_ROOT" ]] || { echo "REFUSED: release root already exists: $RELEASE_ROOT" >&2; exit 2; }
else
    RELEASE_ROOT="$(mktemp -d /tmp/abastevo-portal-release.XXXXXX)"
    ROOT_TEMP=1
fi

log() { echo "rehearse-portal: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 1; }

cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    if [[ "$ROOT_TEMP" == "1" && "$KEEP" != "1" ]]; then
        rm -rf "$RELEASE_ROOT"
    fi
    rm -rf "$WORK"
}
trap cleanup EXIT

[[ -f "$ARTIFACT_A/portal-manifest.json" ]] || refuse "artifact A is not a portal candidate"
[[ -z "$ARTIFACT_B" || -f "$ARTIFACT_B/portal-manifest.json" ]] || refuse "artifact B is not a portal candidate"
[[ -f "$CADDYFILE" ]] || refuse "rehearsal Caddyfile missing"
docker image inspect "$CADDY_IMAGE" >/dev/null 2>&1 || refuse "pinned Caddy image is not available locally"

(cd backend && go build -o "$BIN" ./cmd/portalrelease)
docker run --rm -v "$CADDYFILE:/etc/caddy/Caddyfile:ro" "$CADDY_IMAGE" \
    caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1

release_hash() {
    "$BIN" verify --root "$RELEASE_ROOT" --site-url "$SITE_URL" | sed -n 's/^current=\([^ ]*\).*/\1/p'
}

wait_http() {
    for _ in $(seq 1 100); do
        if curl -sS -o /dev/null --max-time 1 "$ORIGIN/consultar/" 2>/dev/null; then
            return 0
        fi
        sleep 0.1
    done
    docker logs "$NAME" >&2 || true
    refuse "Caddy did not become ready"
}

fetch() {
    local url="$1" output="$2" headers="$3"
    shift 3
    curl -sS --max-time 5 -D "$headers" -o "$output" "$@" "$url"
}

status() {
    local url="$1"
    curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$url"
}

header_value() {
    local headers="$1" name="$2"
    awk -v key="$name" '
        index(tolower($0), tolower(key) ":") == 1 {
            line=$0
            sub(/^[^:]*:[[:space:]]*/, "", line)
            sub(/\r$/, "", line)
            print line
            exit
        }' "$headers"
}

assert_status() {
    local desc="$1" want="$2" url="$3" got
    got="$(status "$url")"
    [[ "$got" == "$want" ]] || refuse "$desc: got $got want $want"
    log "$desc: $got"
}

assert_header() {
    local desc="$1" headers="$2" name="$3" want="$4" got
    got="$(header_value "$headers" "$name")"
    [[ "$got" == *"$want"* ]] || refuse "$desc: $name=$got want $want"
    log "$desc: $name=$got"
}

assert_body() {
    local desc="$1" body="$2" want="$3"
    grep -qF "$want" "$body" || refuse "$desc: body missing $want"
    log "$desc: body ok"
}

assert_gzip() {
    local desc="$1" url="$2"
    fetch "$url" "$WORK/body.gz" "$WORK/headers.gz" -H 'Accept-Encoding: gzip'
    assert_header "$desc encoding" "$WORK/headers.gz" Content-Encoding gzip
    assert_header "$desc type" "$WORK/headers.gz" Content-Type "$3"
}

log "promoting candidate A"
"$BIN" promote --artifact "$ARTIFACT_A" --root "$RELEASE_ROOT" --site-url "$SITE_URL" --fence --expect-current "" >/dev/null
A_HASH="$(release_hash)"
[[ -n "$A_HASH" ]] || refuse "candidate A did not become current"

log "starting isolated Caddy on $ORIGIN"
docker run --rm -d --name "$NAME" \
    -p "127.0.0.1:${HTTP_PORT}:80" \
    -v "$CADDYFILE:/etc/caddy/Caddyfile:ro" \
    -v "$RELEASE_ROOT:/srv/portal:ro" \
    "$CADDY_IMAGE" >/dev/null
wait_http

assert_status "consultar" 200 "$ORIGIN/consultar/"
fetch "$ORIGIN/consultar/" "$WORK/consultar.html" "$WORK/consultar.headers"
assert_header "consultar type" "$WORK/consultar.headers" Content-Type text/html
assert_header "consultar cache" "$WORK/consultar.headers" Cache-Control "public, max-age=0, must-revalidate"
assert_header "consultar nosniff" "$WORK/consultar.headers" X-Content-Type-Options nosniff
assert_header "consultar robots" "$WORK/consultar.headers" X-Robots-Tag noindex
assert_gzip "consultar" "$ORIGIN/consultar/" text/html
assert_status "missing page" 404 "$ORIGIN/nao-existe/"
fetch "$ORIGIN/nao-existe/" "$WORK/404.html" "$WORK/404.headers"
assert_body "404" "$WORK/404.html" "Página não encontrada"
assert_status "no API proxy" 404 "$ORIGIN/v1/health/live"
assert_status "no health proxy" 404 "$ORIGIN/health/live"

CSS_PATH="$(sed -n 's/.*href="\(\/assets\/[^"]*\.css\)".*/\1/p' "$WORK/consultar.html" | head -1)"
[[ -n "$CSS_PATH" ]] || refuse "consultar has no stylesheet"
assert_status "css" 200 "$ORIGIN$CSS_PATH"
fetch "$ORIGIN$CSS_PATH" "$WORK/css" "$WORK/css.headers" -H 'Accept-Encoding: gzip'
assert_header "css cache" "$WORK/css.headers" Cache-Control "public, max-age=31536000, immutable"
assert_gzip "css" "$ORIGIN$CSS_PATH" text/css

if [[ -n "$ARTIFACT_B" ]]; then
    log "promoting candidate B with fence"
    "$BIN" promote --artifact "$ARTIFACT_B" --root "$RELEASE_ROOT" --site-url "$SITE_URL" --fence --expect-current "$A_HASH" >/dev/null
    B_HASH="$(release_hash)"
    [[ -n "$B_HASH" && "$B_HASH" != "$A_HASH" ]] || refuse "candidate B did not switch"
    assert_status "B sitemap" 200 "$ORIGIN/sitemap.xml"
    fetch "$ORIGIN/consultar/" "$WORK/consultar-b.html" "$WORK/consultar-b.headers"
    grep -q 'portal\.[a-f0-9]*\.js' "$WORK/consultar-b.html" || refuse "B search script missing"
    JS_PATH="$(sed -n 's/.*src="\(\/assets\/portal\.[a-f0-9]*\.js\)".*/\1/p' "$WORK/consultar-b.html" | head -1)"
    [[ -n "$JS_PATH" ]] || refuse "B script path missing"
    assert_gzip "js" "$ORIGIN$JS_PATH" javascript

    log "rolling back to candidate A"
    "$BIN" rollback --root "$RELEASE_ROOT" --target "$A_HASH" --site-url "$SITE_URL" >/dev/null
    assert_status "A sitemap after rollback" 404 "$ORIGIN/sitemap.xml"

    log "corrupting current A and proving safe rollback to B"
    CORRUPT="$(find "$RELEASE_ROOT/releases/$(echo "$A_HASH" | sed 's/^sha256://')" -type f -name '*.html' -print -quit)"
    [[ -n "$CORRUPT" ]] || refuse "no release page to corrupt"
    printf 'corrupt' >"$CORRUPT"
    if "$BIN" verify --root "$RELEASE_ROOT" --site-url "$SITE_URL" >/dev/null 2>&1; then
        refuse "corrupt current passed verification"
    fi
    "$BIN" rollback --root "$RELEASE_ROOT" --target "$B_HASH" --site-url "$SITE_URL" >/dev/null
    assert_status "B sitemap after corrupt rollback" 200 "$ORIGIN/sitemap.xml"

    log "re-promoting A and withdrawing A to B"
    "$BIN" promote --artifact "$ARTIFACT_A" --root "$RELEASE_ROOT" --site-url "$SITE_URL" --fence --expect-current "$B_HASH" >/dev/null
    "$BIN" withdraw --root "$RELEASE_ROOT" --fallback "$B_HASH" --site-url "$SITE_URL" >/dev/null
    assert_status "B sitemap after withdraw to B" 200 "$ORIGIN/sitemap.xml"
    if "$BIN" rollback --root "$RELEASE_ROOT" --target "$A_HASH" --site-url "$SITE_URL" >/dev/null 2>&1; then
        refuse "withdrawn A rolled back"
    fi

    log "withdrawing the current release without fallback"
    "$BIN" withdraw --root "$RELEASE_ROOT" --site-url "$SITE_URL" >/dev/null
    assert_status "withdrawn portal" 404 "$ORIGIN/consultar/"
fi

if [[ "$KEEP" == "1" ]]; then
    log "release root retained: $RELEASE_ROOT"
fi
echo "rehearse-portal ok: isolated static root, atomic switch/rollback, headers/compression/404 and API independence"
