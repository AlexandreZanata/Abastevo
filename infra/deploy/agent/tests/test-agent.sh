#!/usr/bin/env bash
# Deterministic failure-behavior harness for the staging deploy planner.
# Stubs oras, cosign, curl and deployment tools; no network and no mutation.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
AGENT="$ROOT/infra/deploy/agent/abastevo-deploy-agent"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

STUB_BIN="$TMP_ROOT/bin"
mkdir -p "$STUB_BIN"

cat > "$STUB_BIN/oras" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "${ORAS_FAIL:-0}" == "0" ]] || exit 1
[[ -n "${ORAS_FIXTURE_DIR:-}" ]] || exit 1
case "${1:-}" in
    manifest)
        case "${2:-}" in
            fetch)
                if [[ "${3:-}" == "--descriptor" ]]; then
                    cat "$ORAS_FIXTURE_DIR/descriptor.json"
                else
                    cat "$ORAS_FIXTURE_DIR/oci-manifest.json"
                fi
                ;;
            *) exit 1 ;;
        esac
        ;;
    pull)
        [[ "${2:-}" == "--output" ]] || exit 1
        out="${3:?}"
        mkdir -p "$out"
        cp "$ORAS_FIXTURE_DIR"/artifact/* "$out"/
        ;;
    *) exit 1 ;;
esac
STUB

cat > "$STUB_BIN/cosign" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
bundle=""
identity=""
issuer=""
while (( $# )); do
    case "$1" in
        --bundle) bundle="${2:-}"; shift 2 ;;
        --certificate-identity) identity="${2:-}"; shift 2 ;;
        --certificate-oidc-issuer) issuer="${2:-}"; shift 2 ;;
        *) shift ;;
    esac
done
[[ -n "$bundle" && -f "$bundle" ]] || exit 1
[[ "$identity" == "https://github.com/AlexandreZanata/Abastevo/.github/workflows/release-staging.yml@refs/heads/main" ]] || exit 1
[[ "$issuer" == "https://token.actions.githubusercontent.com" ]] || exit 1
case "${COSIGN_MODE:-ok}" in
    ok) exit 0 ;;
    tampered|unsigned|foreign) printf 'simulated signature refusal\n' >&2; exit 1 ;;
    *) exit 1 ;;
esac
STUB

cat > "$STUB_BIN/curl" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
out=""
while (( $# )); do
    case "$1" in
        -o) out="${2:-}"; shift 2 ;;
        -w) shift 2 ;;
        --connect-timeout|--max-time|-H) shift 2 ;;
        -*) shift ;;
        *) shift ;;
    esac
done
[[ -n "$out" ]] || exit 1
case "${CURL_MODE:-behind}" in
    behind) printf '{"status":"behind"}\n' > "$out"; printf '200' ;;
    identical) printf '{"status":"identical"}\n' > "$out"; printf '200' ;;
    ahead) printf '{"status":"ahead"}\n' > "$out"; printf '200' ;;
    diverged) printf '{"status":"diverged"}\n' > "$out"; printf '200' ;;
    notfound) printf '{}\n' > "$out"; printf '404' ;;
    ratelimit) printf '{}\n' > "$out"; printf '403' ;;
    fail) exit 22 ;;
    *) exit 1 ;;
esac
STUB

for command in docker kubectl helm systemctl; do
    cat > "$STUB_BIN/$command" <<'STUB'
#!/usr/bin/env bash
: > "${MUTATION_MARKER:?}"
exit 1
STUB
done

chmod +x "$STUB_BIN"/*

PASS=0
FAIL=0
FIXTURE_SHA="1111111111111111111111111111111111111111"

pass() {
    echo "PASS: $1"
    PASS=$((PASS + 1))
}

failed() {
    echo "FAIL: $1" >&2
    FAIL=$((FAIL + 1))
}

write_oci() {
    local dir="$1"
    local manifest_sha bundle_sha
    manifest_sha="sha256:$(sha256sum "$dir/artifact/release-manifest.json" | cut -d' ' -f1)"
    bundle_sha="sha256:$(sha256sum "$dir/artifact/release-manifest.sigstore.json" | cut -d' ' -f1)"
    jq -n --arg md "$manifest_sha" --arg bd "$bundle_sha" '{
        schemaVersion: 2,
        mediaType: "application/vnd.oci.image.manifest.v1+json",
        artifactType: "application/vnd.abastevo.release.v1+json",
        config: {
            mediaType: "application/vnd.oci.empty.v1+json",
            digest: "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
            size: 2
        },
        layers: [
            {mediaType: "application/vnd.abastevo.release.manifest.v1+json", digest: $md, size: 1},
            {mediaType: "application/vnd.dev.sigstore.bundle.v0.3+json", digest: $bd, size: 1}
        ]
    }' > "$dir/oci-manifest.json"
    jq -n --arg digest "sha256:$(printf '0%.0s' {1..64})" '{
        mediaType: "application/vnd.oci.image.manifest.v1+json",
        digest: $digest,
        size: 1
    }' > "$dir/descriptor.json"
}

make_fixture() {
    local dir="$1"
    mkdir -p "$dir/artifact" "$dir/state" "$dir/landing" "$dir/backup" "$dir/receipts"
    jq -n \
        --arg sha "$FIXTURE_SHA" \
        --arg api "ghcr.io/alexandrezanata/abastevo-api@sha256:$(printf '2%.0s' {1..64})" \
        --arg worker "ghcr.io/alexandrezanata/abastevo-worker@sha256:$(printf '3%.0s' {1..64})" \
        --arg migrate "ghcr.io/alexandrezanata/abastevo-migrate@sha256:$(printf '4%.0s' {1..64})" \
        --arg landing "$(printf '5%.0s' {1..64})" \
        '{
            schema: "abastevo-release-manifest-v1",
            sha: $sha,
            created_at: "2026-10-27T00:00:00Z",
            images: {api: $api, worker: $worker, migrate: $migrate},
            landing_sha256: $landing,
            migrations_max: "000132",
            workflow_run_url: "https://github.com/AlexandreZanata/Abastevo/actions/runs/123"
        }' > "$dir/artifact/release-manifest.json"
    printf '{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}\n' \
        > "$dir/artifact/release-manifest.sigstore.json"
    write_oci "$dir"
}

new_case() {
    local name="$1"
    local dir="$TMP_ROOT/$name"
    mkdir -p "$dir"
    make_fixture "$dir"
    printf '%s\n' "$dir"
}

make_current() {
    local dir="$1"
    jq '{
        schema: "abastevo-deploy-current-v1",
        sha: .sha,
        images: .images,
        landing_sha256: .landing_sha256,
        migrations_max: .migrations_max,
        applied_at: "2026-10-27T00:00:00Z"
    }' "$dir/artifact/release-manifest.json" > "$dir/state/current.json"
}

run_agent() {
    local dir="$1"
    (
        export PATH="$STUB_BIN:$PATH"
        export ORAS_FIXTURE_DIR="$dir"
        export ORAS_FAIL="${ORAS_FAIL:-0}"
        export COSIGN_MODE="${COSIGN_MODE:-ok}"
        export CURL_MODE="${CURL_MODE:-behind}"
        export MUTATION_MARKER="$dir/mutation"
        export ABASTEVO_NAMESPACE="${ABASTEVO_NAMESPACE_OVERRIDE-test-ns}"
        export ABASTEVO_API_DEPLOYMENT="api"
        export ABASTEVO_WORKER_DEPLOYMENT="worker"
        export ABASTEVO_MIGRATE_JOB="migrate"
        export ABASTEVO_LANDING_ROOT="$dir/landing"
        export ABASTEVO_BACKUP_DIR="$dir/backup"
        export ABASTEVO_RECEIPT_DIR="$dir/receipts"
        export ABASTEVO_STATE_DIR="$dir/state"
        export ABASTEVO_GITHUB_REPOSITORY="AlexandreZanata/Abastevo"
        export ABASTEVO_GITHUB_API="https://api.github.invalid"
        export ABASTEVO_MANIFEST_REPOSITORY="ghcr.io/alexandrezanata/abastevo-release"
        export ABASTEVO_MANIFEST_TAG="staging"
        "$AGENT"
    ) >"$dir/stdout.log" 2>"$dir/stderr.log"
}

latest_receipt() {
    local dir="$1"
    find "$dir/receipts" -maxdepth 1 -name 'plan-*.json' -type f | sort | tail -n 1
}

expect_success() {
    local name="$1"
    shift
    if "$@"; then
        pass "$name"
    else
        failed "$name (expected success)"
    fi
}

expect_failure() {
    local name="$1"
    shift
    if "$@"; then
        failed "$name (expected failure, got success)"
    else
        pass "$name"
    fi
}

assert_eq() {
    local name="$1" actual="$2" expected="$3"
    if [[ "$actual" == "$expected" ]]; then
        pass "$name"
    else
        failed "$name (got '$actual', expected '$expected')"
    fi
}

assert_absent() {
    local name="$1" path="$2"
    if [[ ! -e "$path" ]]; then
        pass "$name"
    else
        failed "$name (unexpected path exists)"
    fi
}

assert_no_receipt() {
    local name="$1" dir="$2"
    if [[ -z "$(latest_receipt "$dir")" ]]; then
        pass "$name"
    else
        failed "$name (unexpected receipt)"
    fi
}

echo "== new release writes a plan-only receipt without mutating state =="
CASE="$(new_case new-release)"
expect_success "new release accepted" run_agent "$CASE"
RECEIPT="$(latest_receipt "$CASE")"
if [[ -n "$RECEIPT" ]]; then
    pass "new release receipt exists"
    assert_eq "new release action" "$(jq -r '.action' "$RECEIPT")" "plan-only"
    assert_eq "new release change flag" "$(jq -r '.changes.release' "$RECEIPT")" "true"
    assert_eq "new release target sha" "$(jq -r '.target.sha' "$RECEIPT")" "$FIXTURE_SHA"
    assert_eq "new release current is null" "$(jq -r '.current' "$RECEIPT")" "null"
    assert_eq "new release signature verified" "$(jq -r '.manifest.signature' "$RECEIPT")" "verified"
    assert_eq "new release ancestry verified" "$(jq -r '.manifest.sha_reachable_from_main' "$RECEIPT")" "true"
    assert_eq "receipt mode" "$(stat -c '%a' "$RECEIPT")" "600"
else
    failed "new release receipt exists"
fi
assert_absent "new release does not create current.json" "$CASE/state/current.json"
assert_absent "new release does not call deployment tools" "$CASE/mutation"

echo "== unchanged release writes a no-change receipt =="
CASE="$(new_case no-change)"
make_current "$CASE"
expect_success "no-change release accepted" run_agent "$CASE"
RECEIPT="$(latest_receipt "$CASE")"
if [[ -n "$RECEIPT" ]]; then
    pass "no-change receipt exists"
    assert_eq "no-change action" "$(jq -r '.action' "$RECEIPT")" "no-change"
    assert_eq "no-change release flag" "$(jq -r '.changes.release' "$RECEIPT")" "false"
    assert_eq "no-change landing flag" "$(jq -r '.changes.landing' "$RECEIPT")" "false"
    assert_eq "no-change migrations flag" "$(jq -r '.changes.migrations' "$RECEIPT")" "false"
else
    failed "no-change receipt exists"
fi
assert_absent "no-change does not call deployment tools" "$CASE/mutation"

echo "== refusals =="
CASE="$(new_case tampered)"; COSIGN_MODE=tampered expect_failure "tampered signature refused" run_agent "$CASE"
assert_no_receipt "tampered signature writes no receipt" "$CASE"

CASE="$(new_case unsigned)"; rm "$CASE/artifact/release-manifest.sigstore.json"
expect_failure "unsigned manifest refused" run_agent "$CASE"

CASE="$(new_case foreign)"; COSIGN_MODE=foreign expect_failure "foreign identity refused" run_agent "$CASE"

CASE="$(new_case ahead)"; CURL_MODE=ahead expect_failure "commit ahead of main refused" run_agent "$CASE"
CASE="$(new_case diverged)"; CURL_MODE=diverged expect_failure "diverged commit refused" run_agent "$CASE"
CASE="$(new_case notfound)"; CURL_MODE=notfound expect_failure "GitHub 404 refused" run_agent "$CASE"
CASE="$(new_case ratelimit)"; CURL_MODE=ratelimit expect_failure "GitHub 403 refused" run_agent "$CASE"
CASE="$(new_case curlfail)"; CURL_MODE=fail expect_failure "GitHub network failure refused" run_agent "$CASE"
CASE="$(new_case orasfail)"; ORAS_FAIL=1 expect_failure "registry failure refused" run_agent "$CASE"

CASE="$(new_case invalid-schema)"
jq '.schema = "wrong"' "$CASE/artifact/release-manifest.json" > "$CASE/manifest.tmp"
mv "$CASE/manifest.tmp" "$CASE/artifact/release-manifest.json"
write_oci "$CASE"
expect_failure "invalid manifest schema refused" run_agent "$CASE"

CASE="$(new_case digest-mismatch)"
printf '\n' >> "$CASE/artifact/release-manifest.json"
expect_failure "manifest digest mismatch refused" run_agent "$CASE"

CASE="$(new_case missing-bundle-layer)"
jq '.layers = [.layers[] | select(.mediaType != "application/vnd.dev.sigstore.bundle.v0.3+json")]' \
    "$CASE/oci-manifest.json" > "$CASE/oci.tmp"
mv "$CASE/oci.tmp" "$CASE/oci-manifest.json"
expect_failure "missing signature layer refused" run_agent "$CASE"

CASE="$(new_case bad-current)"
make_current "$CASE"
printf '{"schema":"bad"}\n' > "$CASE/state/current.json"
expect_failure "invalid current state refused" run_agent "$CASE"

CASE="$(new_case missing-config)"
ABASTEVO_NAMESPACE_OVERRIDE="" expect_failure "missing configuration refused" run_agent "$CASE"

echo "== summary: $PASS passed, $FAIL failed =="
[[ "$FAIL" -eq 0 ]]
