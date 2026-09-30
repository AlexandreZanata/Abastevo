#!/usr/bin/env bash
# Real pinned Caddy + local CA. No public certificate issuance or trust-store
# modification. ANPFUEL_LOCAL_API_URL must select a synthetic local API.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
API_URL="${ANPFUEL_LOCAL_API_URL:?Set the local synthetic API URL}"
# Restrict the upstream to a literal loopback/private IPv4; never a real host.
python3 - "$API_URL" <<'PY'
import ipaddress, sys, urllib.parse
u=urllib.parse.urlparse(sys.argv[1])
a=ipaddress.ip_address(u.hostname)
assert u.scheme == 'http' and (a.is_loopback or a.is_private) and u.port and u.path in ('','/')
PY
WORK="$(mktemp -d /tmp/anpfuel-edge-suite.XXXXXXXX)"
PORT="$(python3 - <<'PY'
import socket
with socket.socket() as s:
 s.bind(('127.0.0.1',0)); print(s.getsockname()[1])
PY
)"
NAME="anpfuel-edge-validation-$(basename "$WORK" | tr '[:upper:].' '[:lower:]-')"
cleanup() { docker rm -fv "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT INT TERM
python3 - "$ROOT/infra/caddy/Caddyfile" "$WORK/Caddyfile" "$API_URL" <<'PY'
import pathlib, sys, urllib.parse
s=pathlib.Path(sys.argv[1]).read_text()
s=s.replace('admin off','admin off\n\tauto_https disable_redirects\n\tskip_install_trust',1)
s=s.replace('tls {','tls internal {',1)
s=s.replace('reverse_proxy api:8080', 'reverse_proxy '+urllib.parse.urlparse(sys.argv[3]).netloc)
s=s.replace('@v1 path /v1/* /health/*', 'handle /v1/validation-upstream-down {\n\t\treverse_proxy 127.0.0.1:9\n\t}\n\t@v1 path /v1/* /health/*')
s=s.replace('{$CADDY_DOMAIN} {','{$CADDY_DOMAIN} {\n\tbind 127.0.0.1',1)
pathlib.Path(sys.argv[2]).write_text(s)
PY
docker run -d --name "$NAME" --network host --memory 256m --cpus 1 \
    --tmpfs /data --tmpfs /config \
    -e "CADDY_DOMAIN=localhost:$PORT" -e CADDY_TLS_EMAIL=local@example.invalid \
    -v "$WORK/Caddyfile:/etc/caddy/Caddyfile:ro" \
    caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d >/dev/null
READY=0
for _ in $(seq 1 30); do
    if docker exec "$NAME" cat /data/caddy/pki/authorities/local/root.crt >"$WORK/root.crt" 2>/dev/null; then READY=1; break; fi
    sleep 1
done
[[ "$READY" == 1 ]] || { echo 'local CA did not start' >&2; exit 1; }
BASE="https://localhost:$PORT"
curl -fsS --cacert "$WORK/root.crt" --tlsv1.2 --tls-max 1.2 "$BASE/health/ready" >"$WORK/ready.json"
curl -fsS --cacert "$WORK/root.crt" --tlsv1.3 "$BASE/health/live" >/dev/null
# Put synthetic private markers in every risky surface; no marker may be logged.
curl -fsS --cacert "$WORK/root.crt" \
    -H 'X-ANPFuel-Signature: synthetic-private-proof-marker' \
    -H 'Cookie: marker=synthetic-private-cookie-marker' \
    "$BASE/v1/stations/nearby?lat=-23.551234&lon=-46.661234&radius_m=1000" >/dev/null
for path in /metrics /admin /not-an-api; do
    code="$(curl -sS --cacert "$WORK/root.crt" -o /dev/null -w '%{http_code}' "$BASE$path")"
    [[ "$code" == 404 ]] || { echo "edge route exposed: $path" >&2; exit 1; }
done
code="$(curl -sS --cacert "$WORK/root.crt" -o /dev/null -w '%{http_code}' \
    -H 'X-ANPFuel-Signature: synthetic-private-proof-marker' \
    "$BASE/v1/validation-upstream-down?lat=-23.551234&lon=-46.661234")"
[[ "$code" == 502 ]] || { echo "upstream failure not surfaced" >&2; exit 1; }
sleep 1
docker logs "$NAME" >"$WORK/caddy.log" 2>&1
if [[ -n "${ANPFUEL_EDGE_REPORT_DIR:-}" ]]; then
    mkdir -p "$ANPFUEL_EDGE_REPORT_DIR"
    cp "$WORK/caddy.log" "$ANPFUEL_EDGE_REPORT_DIR/caddy.log"
fi
python3 - "$WORK/caddy.log" <<'PY'
import json, pathlib, sys
raw=pathlib.Path(sys.argv[1]).read_text()
for marker in ('synthetic-private-proof-marker','synthetic-private-cookie-marker','23.551234','46.661234'):
 assert marker not in raw, 'private marker leaked to edge logs'
access=[json.loads(line) for line in raw.splitlines() if line.startswith('{')]
access=[e for e in access if e.get('logger','').startswith('http.log.access')]
assert len(access)>=6, 'access-log proof missing'
assert all('request' not in e and 'resp_headers' not in e for e in access)
errors=[e for e in [json.loads(line) for line in raw.splitlines() if line.startswith('{')] if e.get('logger','').startswith('http.log.error') or (e.get('level','').lower()=='error' and e.get('status',0)>=500)]
assert errors and all('request' not in e for e in errors), 'runtime failure privacy proof missing'
PY
printf 'Local edge passed: TLS 1.2/1.3, ready/live, private log markers, route isolation.\n'
