#!/usr/bin/env bash
# Reproduce cold-pull stderr without evicting images or touching shared services.
# The complete SEC03C matrices still run against real disposable loopback MinIO.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REAL_DOCKER="$(command -v docker)"
TMP="$(mktemp -d)"
export ANPFUEL_S3_STARTUP_RUN="$(basename "$TMP")"
cleanup() {
    local id
    while IFS= read -r id; do
        [[ -z "$id" ]] || "$REAL_DOCKER" rm -fv "$id" >/dev/null
    done < <("$REAL_DOCKER" ps -aq --filter "label=abastevo.test.s3-startup=$ANPFUEL_S3_STARTUP_RUN")
    rm -rf "$TMP"
}
trap cleanup EXIT
mkdir "$TMP/bin"
{
    cat <<'WRAPPER'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == run && "${2:-}" == -d ]]; then
    echo 'Synthetic cold pull: downloading layer (stderr)' >&2
    shift 2
WRAPPER
    printf '    exec %q run -d --label "abastevo.test.s3-startup=$ANPFUEL_S3_STARTUP_RUN" "$@"\n' "$REAL_DOCKER"
    printf 'fi\nexec %q "$@"\n' "$REAL_DOCKER"
} > "$TMP/bin/docker"
chmod +x "$TMP/bin/docker"
cd "$ROOT/backend"
PATH="$TMP/bin:$PATH" go test -race -count=1 -tags=integration -run TestSEC03C \
    ./internal/modules/evidence/adapters/storage ./internal/modules/evidence/application
if [[ -n "$("$REAL_DOCKER" ps -aq --filter "label=abastevo.test.s3-startup=$ANPFUEL_S3_STARTUP_RUN")" ]]; then
    echo 'FAIL: S3 fixture leaked a container' >&2
    exit 1
fi
echo 'PASS: cold-pull diagnostics preserved and both real S3 fixtures cleaned up'
