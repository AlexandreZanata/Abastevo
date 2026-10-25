#!/usr/bin/env bash
# ESP-16 bounded static-portal capacity and browser drill.
#
# Freezes hardware/workload/budgets, starts the pinned Caddy image on a private
# Docker network, runs a separate pinned Python generator container with the
# existing qualified driver, measures Caddy resources, then runs a headless
# Chrome probe. It never contacts the API, database, ANP or the shared edge.
# Results are local evidence only; CDN and VPS-direct campaigns are separate.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

RELEASE_ROOT="${PORTAL_RELEASE_ROOT:?PORTAL_RELEASE_ROOT is required}"
SITE_URL="${PORTAL_SITE_URL:?PORTAL_SITE_URL is required}"
ORIGIN="${PORTAL_ORIGIN:-http://127.0.0.1:18081}"
HTTP_PORT="${PORTAL_HTTP_PORT:-18081}"
STATION_ID="${PORTAL_STATION_ID:-00000000-0000-4000-8000-000000000001}"
RATE="${PORTAL_RATE:-100}"
DURATION="${PORTAL_DURATION:-30}"
CLIENTS="${PORTAL_CLIENTS:-32}"
DEADLINE_MS="${PORTAL_DEADLINE_MS:-1000}"
MIN_RPS="${PORTAL_MIN_RPS:-90}"
P95="${PORTAL_P95_BUDGET_MS:-50}"
P99="${PORTAL_P99_BUDGET_MS:-100}"
ERR="${PORTAL_ERR_BUDGET:-0.01}"
CPU_BUDGET="${PORTAL_CPU_BUDGET_PCT:-80}"
MEM_BUDGET="${PORTAL_MEM_BUDGET_MIB:-256}"
BROWSER_RUNS="${PORTAL_BROWSER_RUNS:-3}"
FCP_BUDGET="${PORTAL_FCP_BUDGET_MS:-1500}"
LCP_BUDGET="${PORTAL_LCP_BUDGET_MS:-2500}"
CLS_BUDGET="${PORTAL_CLS_BUDGET:-0.1}"
CHROME="${PORTAL_CHROME:-google-chrome}"
UPDATE_ARTIFACT="${PORTAL_UPDATE_ARTIFACT:-}"
UPDATE_AT="${PORTAL_UPDATE_AT_SECONDS:-8}"
ROLLBACK_AT="${PORTAL_ROLLBACK_AT_SECONDS:-16}"
CADDY_IMAGE='caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d'
GENERATOR_IMAGE='python:3.11-slim@sha256:cce12316475d7d56d605f76e1d811d0dc4e7227f2d4afd29951521264f504ff8'
CADDYFILE="$ROOT/infra/caddy/portal-rehearsal.Caddyfile"
RESULTS="${PORTAL_RESULTS_DIR:-/tmp/abastevo-portal-capacity-$(date -u +%Y%m%dT%H%M%SZ)}"
WORK="$(mktemp -d /tmp/abastevo-portal-capacity-work.XXXXXX)"
NAME="abastevo-portal-capacity-$$"
GEN_NAME="abastevo-portal-capacity-gen-$$"
NET="abastevo-portal-capacity-$$-$(date +%s)"
SAMPLER=""
GEN_PID=""

log() { echo "portal-capacity: $*" >&2; }
refuse() { echo "REFUSED: $*" >&2; exit 2; }
cleanup() {
    [[ -n "$SAMPLER" ]] && kill "$SAMPLER" 2>/dev/null || true
    [[ -n "$GEN_PID" ]] && kill "$GEN_PID" 2>/dev/null || true
    docker rm -f "$NAME" "$GEN_NAME" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
    rm -rf "$WORK"
}
trap cleanup EXIT

[[ -f "$CADDYFILE" ]] || refuse "rehearsal Caddyfile missing"
[[ -d "$RELEASE_ROOT" ]] || refuse "release root missing"
[[ -f "$RELEASE_ROOT/current/consultar/index.html" ]] || refuse "release root has no current portal"
command -v curl >/dev/null || refuse "curl required"
command -v python3 >/dev/null || refuse "python3 required"
command -v node >/dev/null || refuse "node required"
command -v "$CHROME" >/dev/null || refuse "chrome required"
docker image inspect "$CADDY_IMAGE" >/dev/null 2>&1 || refuse "pinned Caddy image unavailable"
docker image inspect "$GENERATOR_IMAGE" >/dev/null 2>&1 || refuse "pinned generator image unavailable"
mkdir -p "$RESULTS"
chmod 700 "$RESULTS"

(cd backend && go build -o "$WORK/portalrelease" ./cmd/portalrelease)
"$WORK/portalrelease" verify --root "$RELEASE_ROOT" --site-url "$SITE_URL" | tee "$RESULTS/release.txt" >/dev/null
RELEASE_LINE="$(cat "$RESULTS/release.txt")"
log "verified release: $RELEASE_LINE"

CURRENT="$RELEASE_ROOT/current"
CSS_PATH="$(sed -n 's/.*href="\(\/assets\/[^"]*\.css\)".*/\1/p' "$CURRENT/consultar/index.html" | head -1)"
JS_PATH="$(sed -n 's/.*src="\(\/assets\/portal\.[^"]*\.js\)".*/\1/p' "$CURRENT/consultar/index.html" | head -1)"
SHARD="$(find "$CURRENT/dados/busca" -type f -name 'SP.json' -print -quit || true)"
CSV="$(find "$CURRENT/dados" -type f -name 'estacoes.csv' -print -quit || true)"
STATION_PAGE="$CURRENT/postos/$STATION_ID/index.html"
SITEMAP="$CURRENT/sitemap.xml"
[[ -n "$CSS_PATH" && -f "$CURRENT$CSS_PATH" ]] || refuse "styles asset missing"
[[ -n "$SHARD" && -f "$SHARD" ]] || refuse "SP search shard missing"
[[ -n "$CSV" && -f "$CSV" ]] || refuse "station CSV missing"
[[ -f "$STATION_PAGE" ]] || refuse "station page missing"
SHARD_PATH="${SHARD#"$CURRENT"}"
CSV_PATH="${CSV#"$CURRENT"}"

PATHS=("/consultar/" "/postos/$STATION_ID/" "$CSS_PATH" "$SHARD_PATH" "$CSV_PATH")
WEIGHTS=(8 4 3 2 1)
if [[ -n "$JS_PATH" && -f "$CURRENT$JS_PATH" ]]; then
    PATHS+=("$JS_PATH")
    WEIGHTS+=(2)
fi
if [[ -f "$SITEMAP" ]]; then
    PATHS+=("/sitemap.xml")
    WEIGHTS+=(1)
fi
: > "$RESULTS/urls-host.txt"
: > "$RESULTS/urls-container.txt"
for i in "${!PATHS[@]}"; do
    for _ in $(seq 1 "${WEIGHTS[$i]}"); do
        echo "${ORIGIN}${PATHS[$i]}" >> "$RESULTS/urls-host.txt"
        echo "http://caddy${PATHS[$i]}" >> "$RESULTS/urls-container.txt"
    done
done
if [[ -n "$UPDATE_ARTIFACT" ]]; then
    UPDATE_PATHS=("/consultar/" "/postos/$STATION_ID/" "$CSS_PATH" "$SHARD_PATH" "$CSV_PATH")
    UPDATE_WEIGHTS=(8 4 2 2 1)
    : > "$RESULTS/urls-update-container.txt"
    for i in "${!UPDATE_PATHS[@]}"; do
        for _ in $(seq 1 "${UPDATE_WEIGHTS[$i]}"); do
            echo "http://caddy${UPDATE_PATHS[$i]}" >> "$RESULTS/urls-update-container.txt"
        done
    done
    [[ -f "$UPDATE_ARTIFACT/portal-manifest.json" ]] || refuse "update artifact is not a portal candidate"
    [[ "$UPDATE_AT" =~ ^[0-9]+$ && "$ROLLBACK_AT" =~ ^[0-9]+$ ]] || refuse "update timing must be integer seconds"
    (( ROLLBACK_AT > UPDATE_AT )) || refuse "rollback must follow promotion"
    (( ROLLBACK_AT < DURATION )) || refuse "rollback must occur inside the load window"
    log "update mix: ${UPDATE_WEIGHTS[*]} (HTML/CSS/shard/CSV; JS and sitemap excluded across the switch)"
fi
if [[ -n "$UPDATE_ARTIFACT" ]]; then
    ACTIVE_MIX='{"consultar":8,"station":4,"css":2,"search_shard":2,"station_csv":1}'
else
    ACTIVE_MIX="{\"consultar\":8,\"station\":4,\"css\":3,\"search_shard\":2,\"station_csv\":1"
    if [[ -n "$JS_PATH" && -f "$CURRENT$JS_PATH" ]]; then
        ACTIVE_MIX+=',"js":2'
    fi
    if [[ -f "$SITEMAP" ]]; then
        ACTIVE_MIX+=',"sitemap":1'
    fi
    ACTIVE_MIX+='}'
fi
log "url mix: ${WEIGHTS[*]} (host and container); active=$ACTIVE_MIX"

python3 - "$RESULTS" "$ROOT" "$RELEASE_LINE" "$ORIGIN" "$SITE_URL" "$RATE" "$DURATION" "$CLIENTS" "$DEADLINE_MS" "$P95" "$P99" "$ERR" "$CPU_BUDGET" "$MEM_BUDGET" "$FCP_BUDGET" "$LCP_BUDGET" "$CLS_BUDGET" "$MIN_RPS" "$CHROME" "$CADDY_IMAGE" "$GENERATOR_IMAGE" "$UPDATE_ARTIFACT" "$UPDATE_AT" "$ROLLBACK_AT" "$ACTIVE_MIX" <<'PY'
import json, os, platform, re, shutil, subprocess, sys
results, root, release_line, origin, site_url = sys.argv[1:6]
rate, duration, clients, deadline = sys.argv[6:10]
p95, p99, err, cpu, mem, fcp, lcp, cls, min_rps = sys.argv[10:19]
chrome, caddy_image, generator_image, update_artifact = sys.argv[19:23]
update_at, rollback_at, active_mix = sys.argv[23:26]

def size_from_release(key):
    match = re.search(r"\b%s=([0-9]+)" % key, release_line)
    return int(match.group(1)) if match else None

content = re.search(r"current=(\S+)", release_line)
payload = {
    "schema_version": "abastevo-portal-workload-v1",
    "created_at": subprocess.check_output(["date", "-u", "+%Y-%m-%dT%H:%M:%SZ"]).decode().strip(),
    "git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"]).decode().strip(),
    "artifact": {
        "content_sha256": content.group(1) if content else None,
        "pages": size_from_release("pages"),
        "files": size_from_release("files"),
        "bytes": size_from_release("bytes"),
        "site_url": site_url,
    },
    "hardware": {
        "nproc": os.cpu_count(),
        "cpu_model": next((line.split(":", 1)[1].strip() for line in open("/proc/cpuinfo") if line.startswith("model name")), "unknown"),
        "uname": platform.platform(),
        "mem_total_mib": next(int(line.split()[1]) // 1024 for line in open("/proc/meminfo") if line.startswith("MemTotal")),
        "mem_available_mib": next(int(line.split()[1]) // 1024 for line in open("/proc/meminfo") if line.startswith("MemAvailable")),
        "results_disk_free_mib": shutil.disk_usage(results).free // (1024 ** 2),
    },
    "workload": {
        "origin": origin,
        "mode": "open",
        "rate_rps": float(rate),
        "duration_s": float(duration),
        "clients": int(clients),
        "deadline_ms": int(deadline),
        "accept_encoding": "gzip",
        "url_mix": json.loads(active_mix) if active_mix else {},
    },
    "budgets": {
        "p95_ms": float(p95), "p99_ms": float(p99), "err_rate": float(err),
        "cpu_pct": float(cpu), "mem_mib": float(mem),
        "fcp_ms": float(fcp), "lcp_ms": float(lcp), "cls": float(cls),
        "min_rps": float(min_rps),
    },
    "images": {"caddy": caddy_image, "generator": generator_image},
    "browser": subprocess.check_output([chrome, "--version"]).decode().strip(),
    "update": ({
        "artifact": update_artifact,
        "promote_at_s": float(update_at),
        "rollback_at_s": float(rollback_at),
        "mode": "atomic switch then fence-protected rollback; HTML/CSS/shard/CSV mix",
    } if update_artifact else None),
    "scope": "isolated local container drill; separate generator container; host headless Chrome; no API/DB/ANP; not CDN/VPS capacity",
}
with open(os.path.join(results, "workload.json"), "w", encoding="utf-8") as handle:
    json.dump(payload, handle, indent=2)
    handle.write("\n")
os.chmod(os.path.join(results, "workload.json"), 0o600)
PY

log "starting private Caddy container"
docker network create "$NET" >/dev/null
docker run --rm -d --name "$NAME" --network "$NET" --network-alias caddy \
    -p "127.0.0.1:${HTTP_PORT}:80" \
    -v "$CADDYFILE:/etc/caddy/Caddyfile:ro" \
    -v "$RELEASE_ROOT:/srv/portal:ro" \
    "$CADDY_IMAGE" >/dev/null
READY=0
for _ in $(seq 1 100); do
    if [[ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 2 "$ORIGIN/consultar/" 2>/dev/null || true)" == "200" ]]; then
        READY=1
        break
    fi
    sleep 0.1
done
[[ "$READY" == "1" ]] || { docker logs "$NAME" >&2 || true; refuse "Caddy did not become ready"; }
log "Caddy ready on $ORIGIN"

semantic() {
    local path="$1" expect="$2" body
    body="$(curl -fsS --max-time 5 "$ORIGIN$path")" || refuse "semantic fetch failed: $path"
    grep -qF "$expect" <<<"$body" || refuse "semantic body mismatch: $path"
}
semantic "/consultar/" "abastevo"
semantic "/postos/$STATION_ID/" "Posto Alfa"
semantic "$CSS_PATH" "font-family"
if [[ -n "$JS_PATH" && -f "$CURRENT$JS_PATH" ]]; then
    grep -qF "setupPortalSearch" <<<"$(curl -fsS --max-time 5 "$ORIGIN$JS_PATH")" || refuse "search script content mismatch"
fi
semantic "$SHARD_PATH" '"schema":"abastevo-search-v1"'
semantic "$CSV_PATH" "station_id,"
if [[ -f "$SITEMAP" ]]; then
    semantic "/sitemap.xml" "<urlset"
fi
CODE="$(curl -sS -o "$RESULTS/404.html" -w '%{http_code}' --max-time 5 "$ORIGIN/nao-existe/")"
[[ "$CODE" == "404" ]] && grep -qF "Página não encontrada" "$RESULTS/404.html" || refuse "custom 404 missing"
CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$ORIGIN/v1/health/live")"
[[ "$CODE" == "404" ]] || refuse "API path unexpectedly served: $CODE"
curl -sS -D "$RESULTS/headers.txt" -o /dev/null -H 'Accept-Encoding: gzip' "$ORIGIN$CSS_PATH"
grep -qi '^content-encoding: gzip' "$RESULTS/headers.txt" || refuse "gzip not served"
grep -qi '^cache-control: public, max-age=31536000, immutable' "$RESULTS/headers.txt" || refuse "immutable asset cache header missing"
curl -sS -D "$RESULTS/html-headers.txt" -o /dev/null "$ORIGIN/consultar/"
grep -qi '^cache-control: public, max-age=0, must-revalidate' "$RESULTS/html-headers.txt" || refuse "document revalidation header missing"
grep -qi '^x-robots-tag: noindex' "$RESULTS/html-headers.txt" || refuse "rehearsal noindex header missing"
log "semantic checks: PASS"

( while [[ ! -f "$RESULTS/.stop-sampler" ]]; do
      docker stats --no-stream --format '{{.CPUPerc}}|{{.MemUsage}}|{{.NetIO}}|{{.BlockIO}}' "$NAME" >> "$RESULTS/docker-stats.txt" || true
      sleep 1
  done ) &
SAMPLER=$!

URL_FILE="/results/urls-container.txt"
if [[ -n "$UPDATE_ARTIFACT" ]]; then
    URL_FILE="/results/urls-update-container.txt"
fi
log "running generator: ${DURATION}s open-loop at ${RATE} rps, ${CLIENTS} clients"
docker run --rm --name "$GEN_NAME" --network "$NET" --user "$(id -u):$(id -g)" \
    -v "$ROOT/infra/scripts/load:/load:ro" -v "$RESULTS:/results" \
    "$GENERATOR_IMAGE" \
    python3 /load/drive.py --urls-file "$URL_FILE" --mode open \
    --rate "$RATE" --clients "$CLIENTS" --duration "$DURATION" --deadline-ms "$DEADLINE_MS" \
    --gzip --out /results/samples.csv > "$RESULTS/driver.log" 2>&1 &
GEN_PID=$!
if [[ -n "$UPDATE_ARTIFACT" ]]; then
    ORIGINAL_HASH="$(sed -n 's/.*current=\([^ ]*\).*/\1/p' "$RESULTS/release.txt" | head -1)"
    [[ -n "$ORIGINAL_HASH" ]] || refuse "cannot determine original release hash"
    sleep "$UPDATE_AT"
    log "concurrent update: promoting $(basename "$UPDATE_ARTIFACT") with fence $ORIGINAL_HASH"
    "$WORK/portalrelease" promote --artifact "$UPDATE_ARTIFACT" --root "$RELEASE_ROOT" \
        --site-url "$SITE_URL" --fence --expect-current "$ORIGINAL_HASH" | tee "$RESULTS/update.txt"
    UPDATE_HASH="$(sed -n 's/.*current=\([^ ]*\).*/\1/p' "$RESULTS/update.txt" | head -1)"
    [[ -n "$UPDATE_HASH" && "$UPDATE_HASH" != "$ORIGINAL_HASH" ]] || refuse "update did not switch release"
    sleep "$((ROLLBACK_AT - UPDATE_AT))"
    log "concurrent update: rolling back to $ORIGINAL_HASH"
    "$WORK/portalrelease" rollback --root "$RELEASE_ROOT" --target "$ORIGINAL_HASH" \
        --site-url "$SITE_URL" | tee "$RESULTS/update-rollback.txt"
fi
set +e
wait "$GEN_PID"
GEN_STATUS=$?
set -e
GEN_PID=""
cat "$RESULTS/driver.log" | tee /dev/stderr >/dev/null
[[ "$GEN_STATUS" == "0" ]] || refuse "generator failed with $GEN_STATUS"
if [[ -n "$UPDATE_ARTIFACT" ]]; then
    [[ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$ORIGIN/consultar/")" == "200" ]] || refuse "portal not healthy after rollback"
fi
touch "$RESULTS/.stop-sampler"
wait "$SAMPLER" 2>/dev/null || true
SAMPLER=""
rm -f "$RESULTS/.stop-sampler"

python3 infra/scripts/load/stats.py --samples "$RESULTS/samples.csv" > "$RESULTS/stats.txt" 2> "$RESULTS/stats.err" || true
[[ -s "$RESULTS/stats.txt" ]] || { cat "$RESULTS/stats.err" >&2; refuse "stats summary unavailable"; }
log "load stats: $(cat "$RESULTS/stats.txt")"

log "running headless Chrome probe"
node infra/scripts/load/browser_probe.mjs --origin "$ORIGIN" --station "$STATION_ID" \
    --runs "$BROWSER_RUNS" --out "$RESULTS/browser.json" | tee "$RESULTS/browser.log"

if ! python3 infra/scripts/load/portal_report.py --results "$RESULTS" \
    --p95 "$P95" --p99 "$P99" --err "$ERR" --cpu "$CPU_BUDGET" --mem "$MEM_BUDGET" \
    --fcp "$FCP_BUDGET" --lcp "$LCP_BUDGET" --cls "$CLS_BUDGET" --min-rps "$MIN_RPS"; then
    log "campaign failed; artifacts retained at $RESULTS"
    exit 1
fi
log "campaign PASS; artifacts at $RESULTS"
echo "portal-capacity ok: $RESULTS"
