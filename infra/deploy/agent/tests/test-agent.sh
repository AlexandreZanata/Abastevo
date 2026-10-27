#!/usr/bin/env bash
# Deterministic failure-behavior harness for the staging deploy planner.
# shellcheck disable=SC2030,SC2031
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
url=""
want_code=0
while (( $# )); do
    case "$1" in
        -o) out="${2:-}"; shift 2 ;;
        -w) want_code=1; shift 2 ;;
        --connect-timeout|--max-time|-H) shift 2 ;;
        -*) shift ;;
        *) url="$1"; shift ;;
    esac
done
[[ -n "$out" ]] || exit 1
if [[ "$url" == *"/compare/"* ]]; then
    case "${CURL_MODE:-behind}" in
        behind) printf '{"status":"behind"}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '200' ;;
        identical) printf '{"status":"identical"}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '200' ;;
        ahead) printf '{"status":"ahead"}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '200' ;;
        diverged) printf '{"status":"diverged"}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '200' ;;
        notfound) printf '{}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '404' ;;
        ratelimit) printf '{}\n' > "$out"; [[ "$want_code" == "1" ]] && printf '403' ;;
        fail) exit 22 ;;
        *) exit 1 ;;
    esac
    exit 0
fi
case "${SMOKE_MODE:-ok}" in
    fail) exit 22 ;;
    ok)
        case "$url" in
            */portal-manifest.json) printf '{"files":[{"path":"dados/busca/test/SP.json"}]}\n' > "$out" ;;
            *) printf 'ok\n' > "$out" ;;
        esac
        ;;
    *) exit 1 ;;
esac
STUB

for command in docker helm systemctl; do
    cat > "$STUB_BIN/$command" <<'STUB'
#!/usr/bin/env bash
: > "${MUTATION_MARKER:?}"
exit 1
STUB
done

cat > "$STUB_BIN/kubectl" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >> "${KUBECTL_LOG:?}"
printf '\n' >> "$KUBECTL_LOG"
args=("$@")
cmd=()
i=0
while (( i < ${#args[@]} )); do
    if [[ "${args[$i]}" == "-n" ]]; then i=$((i + 2)); continue; fi
    cmd+=("${args[$i]}")
    i=$((i + 1))
done
case "${cmd[0]:-}" in
    get)
        case "${cmd[1]:-}" in
            deployment)
                jq -n --arg schema "${KUBECTL_SCHEMA_ANNOTATION:-000132}" \
                    --argjson maxSurge "${KUBECTL_MAX_SURGE:-0}" \
                    --argjson maxUnavailable "${KUBECTL_MAX_UNAVAILABLE:-1}" \
                    '{metadata:{annotations:{"abastevo.com/schema-version":$schema}},spec:{strategy:{rollingUpdate:{maxSurge:$maxSurge,maxUnavailable:$maxUnavailable}},template:{spec:{containers:[{name:"api",image:"local-api",imagePullPolicy:"Never"},{name:"worker",image:"local-worker",imagePullPolicy:"Never"}]}}}}'
                ;;
            resourcequota)
                jq -n --arg hard "${KUBECTL_QUOTA_HARD:-6}" --arg used "${KUBECTL_QUOTA_USED:-3500m}" \
                    '{status:{hard:{"limits.cpu":$hard},used:{"limits.cpu":$used}}}'
                ;;
            *) exit 1 ;;
        esac
        ;;
    apply)
        tmp="${KUBECTL_JOBS_DIR:?}/.in.$$"
        cat > "$tmp"
        name="$(sed -n 's/^  name: //p' "$tmp" | head -n1)"
        role="$(grep -o 'abastevo.com/role: [a-z]*' "$tmp" | head -n1 | awk '{print $2}')"
        [[ -n "$name" && -n "$role" ]] || { rm -f "$tmp"; exit 1; }
        mv "$tmp" "$KUBECTL_JOBS_DIR/$name.yaml"
        printf '%s\n' "$role" > "$KUBECTL_JOBS_DIR/$name.role"
        [[ "${KUBECTL_APPLY_FAIL:-0}" == "0" ]] || exit 1
        if [[ "$role" == "backup" ]]; then
            backup_dir="$(sed -n 's/^ *path: //p' "$KUBECTL_JOBS_DIR/$name.yaml" | tail -n1)"
            backup_name="$(sed -n 's/.*out="\/backup\/\([^"]*\)\.dump".*/\1/p' "$KUBECTL_JOBS_DIR/$name.yaml" | head -n1)"
            if [[ -n "$backup_dir" && -n "$backup_name" ]]; then
                mkdir -p "$backup_dir"
                printf 'fake dump %s\n' "$backup_name" > "$backup_dir/$backup_name.dump"
                if [[ "${KUBECTL_BACKUP_BAD_HASH:-0}" == "1" ]]; then
                    printf '%064d\n' 0 > "$backup_dir/$backup_name.dump.sha256"
                else
                    sha256sum "$backup_dir/$backup_name.dump" | cut -d' ' -f1 > "$backup_dir/$backup_name.dump.sha256"
                fi
            fi
        fi
        ;;
    wait)
        job="${cmd[-1]#job/}"
        role="$(cat "${KUBECTL_JOBS_DIR:?}/$job.role" 2>/dev/null || true)"
        [[ "${KUBECTL_FAIL_ROLE:-}" != "$role" ]] || exit 1
        ;;
    logs)
        job="${cmd[-1]#job/}"
        role="$(cat "${KUBECTL_JOBS_DIR:?}/$job.role" 2>/dev/null || true)"
        case "$role" in
            schema) printf '%s\n' "${KUBECTL_SCHEMA_DB:-000132}" ;;
            migrate) printf '%s\n' "${KUBECTL_MIGRATE_LOG:-anpfuel migrate: applied [000133]}" ;;
            backup) : ;;
            *) exit 1 ;;
        esac
        ;;
    set)
        [[ "${KUBECTL_SET_IMAGE_FAIL:-0}" == "0" ]] || exit 1
        ;;
    patch)
        [[ "${KUBECTL_PATCH_FAIL:-0}" == "0" ]] || exit 1
        ;;
    rollout)
        case "${cmd[1]:-}" in
            status) [[ "${KUBECTL_ROLLOUT_FAIL:-0}" == "0" ]] || exit 1 ;;
            undo) ;;
            *) exit 1 ;;
        esac
        ;;
    *) exit 1 ;;
esac
STUB

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
    local manifest_sha bundle_sha landing_sha
    manifest_sha="sha256:$(sha256sum "$dir/artifact/release-manifest.json" | cut -d' ' -f1)"
    bundle_sha="sha256:$(sha256sum "$dir/artifact/release-manifest.sigstore.json" | cut -d' ' -f1)"
    landing_sha="sha256:$(sha256sum "$dir/artifact/landing-dist.tar.gz" | cut -d' ' -f1)"
    jq -n --arg md "$manifest_sha" --arg bd "$bundle_sha" --arg ld "$landing_sha" '{
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
            {mediaType: "application/vnd.dev.sigstore.bundle.v0.3+json", digest: $bd, size: 1},
            {mediaType: "application/vnd.abastevo.landing.tar+gzip", digest: $ld, size: 1}
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
    mkdir -p "$dir/landing-src"
    printf '<html>landing</html>\n' > "$dir/landing-src/index.html"
    tar -czf "$dir/artifact/landing-dist.tar.gz" -C "$dir/landing-src" .
    local landing
    landing="$(sha256sum "$dir/artifact/landing-dist.tar.gz" | cut -d' ' -f1)"
    jq -n \
        --arg sha "$FIXTURE_SHA" \
        --arg api "ghcr.io/alexandrezanata/abastevo-api@sha256:$(printf '2%.0s' {1..64})" \
        --arg worker "ghcr.io/alexandrezanata/abastevo-worker@sha256:$(printf '3%.0s' {1..64})" \
        --arg migrate "ghcr.io/alexandrezanata/abastevo-migrate@sha256:$(printf '4%.0s' {1..64})" \
        --arg landing "$landing" \
        '{
            schema: "abastevo-release-manifest-v1",
            sha: $sha,
            created_at: "2026-10-27T00:00:00Z",
            images: {api: $api, worker: $worker, migrate: $migrate},
            landing_sha256: $landing,
            migrations_max: "000132",
            schema_min_supported: "000132",
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

FIXTURE_OLD_SHA="9999999999999999999999999999999999999999"

make_current_old() {
    local dir="$1"
    jq --arg old "$FIXTURE_OLD_SHA" '{
        schema: "abastevo-deploy-current-v1",
        sha: $old,
        images: .images,
        landing_sha256: .landing_sha256,
        migrations_max: .migrations_max,
        schema_min_supported: .schema_min_supported,
        applied_at: "2026-10-27T00:00:00Z"
    }' "$dir/artifact/release-manifest.json" > "$dir/state/current.json"
}

prepare_landing() {
    local dir="$1" count="${2:-0}" i name
    mkdir -p "$dir/landing/releases/$FIXTURE_OLD_SHA"
    printf 'old landing\n' > "$dir/landing/releases/$FIXTURE_OLD_SHA/index.html"
    ln -sfn "releases/$FIXTURE_OLD_SHA" "$dir/landing/current"
    for ((i = 1; i <= count; i++)); do
        name="$(printf '%040d' "$i")"
        mkdir -p "$dir/landing/releases/$name"
        printf 'old landing\n' > "$dir/landing/releases/$name/index.html"
        touch -d "2026-10-0$((i % 9 + 1))" "$dir/landing/releases/$name"
    done
}

run_agent_apply() {
    local dir="$1"
    (
        export PATH="$STUB_BIN:$PATH"
        export ORAS_FIXTURE_DIR="$dir"
        export ORAS_FAIL="${ORAS_FAIL:-0}"
        export COSIGN_MODE="${COSIGN_MODE:-ok}"
        export CURL_MODE="${CURL_MODE:-behind}"
        export SMOKE_MODE="${SMOKE_MODE:-ok}"
        export MUTATION_MARKER="$dir/mutation"
        export ABASTEVO_NAMESPACE="test-ns"
        export ABASTEVO_API_DEPLOYMENT="abastevo-temp-api"
        export ABASTEVO_WORKER_DEPLOYMENT="abastevo-temp-api"
        export ABASTEVO_MIGRATE_JOB="migrate"
        export ABASTEVO_LANDING_ROOT="$dir/landing"
        export ABASTEVO_BACKUP_DIR="$dir/backup"
        export ABASTEVO_RECEIPT_DIR="$dir/receipts"
        export ABASTEVO_STATE_DIR="$dir/state"
        export ABASTEVO_GITHUB_REPOSITORY="AlexandreZanata/Abastevo"
        export ABASTEVO_GITHUB_API="https://api.github.invalid"
        export ABASTEVO_MANIFEST_REPOSITORY="ghcr.io/alexandrezanata/abastevo-release"
        export ABASTEVO_MANIFEST_TAG="staging"
        export ABASTEVO_APPLY=1
        export ABASTEVO_LIBDIR="$ROOT/infra/deploy/agent"
        export ABASTEVO_KUBECTL="$STUB_BIN/kubectl"
        export KUBECONFIG="$dir/kubeconfig"
        export ABASTEVO_SMOKE_BASE_URL="https://staging.invalid"
        export ABASTEVO_SMOKE_TIMEOUT=5
        export ABASTEVO_DB_IMAGE="postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677"
        export ABASTEVO_DB_SECRET="abastevo-temp-secrets"
        export ABASTEVO_ALLOW_SURGE="${ABASTEVO_ALLOW_SURGE:-0}"
        export ABASTEVO_SURGE_CPU_MILLI=2000
        export ABASTEVO_SURGE_RESOURCE_QUOTA="abastevo-temp-quota"
        export ABASTEVO_ROLLOUT_TIMEOUT=30
        export ABASTEVO_JOB_TIMEOUT=30
        export ABASTEVO_BACKUP_KEEP=7
        export ABASTEVO_LANDING_KEEP=5
        export ABASTEVO_LOCK_FILE="$dir/state/apply.lock"
        export KUBECTL_LOG="$dir/kubectl.log"
        export KUBECTL_JOBS_DIR="$dir/jobs"
        export KUBECTL_SCHEMA_ANNOTATION="${KUBECTL_SCHEMA_ANNOTATION:-000132}"
        export KUBECTL_SCHEMA_DB="${KUBECTL_SCHEMA_DB:-000132}"
        export KUBECTL_QUOTA_HARD="${KUBECTL_QUOTA_HARD:-6}"
        export KUBECTL_QUOTA_USED="${KUBECTL_QUOTA_USED:-3500m}"
        export KUBECTL_FAIL_ROLE="${KUBECTL_FAIL_ROLE:-}"
        export KUBECTL_ROLLOUT_FAIL="${KUBECTL_ROLLOUT_FAIL:-0}"
        export KUBECTL_SET_IMAGE_FAIL="${KUBECTL_SET_IMAGE_FAIL:-0}"
        export KUBECTL_PATCH_FAIL="${KUBECTL_PATCH_FAIL:-0}"
        export KUBECTL_APPLY_FAIL="${KUBECTL_APPLY_FAIL:-0}"
        export KUBECTL_BACKUP_BAD_HASH="${KUBECTL_BACKUP_BAD_HASH:-0}"
        mkdir -p "$KUBECTL_JOBS_DIR" "$dir/state"
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

assert_file() {
    local name="$1" path="$2"
    if [[ -f "$path" ]]; then
        pass "$name"
    else
        failed "$name (missing file)"
    fi
}

assert_no_job_role() {
    local name="$1" dir="$2" role="$3"
    if compgen -G "$dir/jobs/*.yaml" >/dev/null \
        && grep -l "abastevo.com/role: $role" "$dir"/jobs/*.yaml >/dev/null 2>&1; then
        failed "$name (unexpected $role job)"
    else
        pass "$name"
    fi
}

job_yaml_for_role() {
    local dir="$1" role="$2"
    grep -l "abastevo.com/role: $role" "$dir"/jobs/*.yaml 2>/dev/null | head -n1
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
    assert_eq "new release schema floor" "$(jq -r '.target.schema_min_supported' "$RECEIPT")" "000132"
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

CASE="$(new_case invalid-schema-floor)"
jq '.schema_min_supported = "132"' "$CASE/artifact/release-manifest.json" > "$CASE/manifest.tmp"
mv "$CASE/manifest.tmp" "$CASE/artifact/release-manifest.json"
write_oci "$CASE"
expect_failure "invalid schema floor refused" run_agent "$CASE"

CASE="$(new_case schema-floor-above-chain)"
jq '.schema_min_supported = "000133"' "$CASE/artifact/release-manifest.json" > "$CASE/manifest.tmp"
mv "$CASE/manifest.tmp" "$CASE/artifact/release-manifest.json"
write_oci "$CASE"
expect_failure "schema floor above migration chain refused" run_agent "$CASE"

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


echo "== apply: successful rollout =="
CASE="$(new_case apply-success)"
make_current_old "$CASE"
prepare_landing "$CASE" 5
for i in $(seq 1 8); do
    printf 'old backup\n' > "$CASE/backup/pre-old$i.dump"
    printf '0000000000000000000000000000000000000000000000000000000000000000\n' \
        > "$CASE/backup/pre-old$i.dump.sha256"
    touch -d "2026-10-0$((i % 9 + 1))" "$CASE/backup/pre-old$i.dump"
done
ABASTEVO_ALLOW_SURGE=1 expect_success "apply success" run_agent_apply "$CASE"
CURRENT="$CASE/state/current.json"
assert_eq "apply current sha" "$(jq -r '.sha' "$CURRENT")" "$FIXTURE_SHA"
assert_eq "apply current floor" "$(jq -r '.schema_min_supported' "$CURRENT")" "000132"
assert_eq "apply current mode" "$(stat -c '%a' "$CURRENT")" "600"
DEPLOY_RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'deploy-*.json' -type f | sort | tail -n 1)"
if [[ -n "$DEPLOY_RECEIPT" ]]; then
    pass "apply success receipt exists"
    assert_eq "apply receipt result" "$(jq -r '.result' "$DEPLOY_RECEIPT")" "deployed"
    assert_eq "apply receipt surge" "$(jq -r '.surge' "$DEPLOY_RECEIPT")" "true"
    assert_eq "apply receipt maxSurge" "$(jq -r '.strategy.maxSurge' "$DEPLOY_RECEIPT")" "1"
    assert_eq "apply receipt gap" "$(jq -r '.strategy.gap' "$DEPLOY_RECEIPT")" "false"
    assert_eq "apply receipt headroom" "$(jq -r '.quota_headroom_milli' "$DEPLOY_RECEIPT")" "2500"
else
    failed "apply success receipt exists"
fi
assert_eq "apply landing target" "$(readlink "$CASE/landing/current")" "releases/$FIXTURE_SHA"
assert_file "apply landing index" "$CASE/landing/releases/$FIXTURE_SHA/index.html"
assert_file "apply previous landing kept" "$CASE/landing/releases/$FIXTURE_OLD_SHA/index.html"
assert_eq "apply landing count" "$(find "$CASE/landing/releases" -mindepth 1 -maxdepth 1 -type d | wc -l)" "5"
assert_eq "apply one shared set image" "$(grep -c 'set image deployment/abastevo-temp-api' "$CASE/kubectl.log")" "1"
assert_eq "apply api digest in set image" "$(grep 'set image' "$CASE/kubectl.log" | grep -c 'abastevo-api@sha256:')" "1"
assert_eq "apply worker digest in set image" "$(grep 'set image' "$CASE/kubectl.log" | grep -c 'abastevo-worker@sha256:')" "1"
assert_eq "apply surge patch" "$(grep -c 'maxSurge' "$CASE/kubectl.log")" "1"
for role in schema backup migrate; do
    job_yaml="$(job_yaml_for_role "$CASE" "$role")"
    assert_file "apply $role job yaml" "$job_yaml"
    if [[ -n "$job_yaml" ]]; then
        assert_eq "apply $role ttl" "$(grep -c 'ttlSecondsAfterFinished: 86400' "$job_yaml")" "1"
        assert_eq "apply $role backoff" "$(grep -c 'backoffLimit: 0' "$job_yaml")" "1"
    fi
done
MIGRATE_YAML="$(job_yaml_for_role "$CASE" migrate)"
BACKUP_YAML="$(job_yaml_for_role "$CASE" backup)"
if [[ -n "$MIGRATE_YAML" ]]; then
    assert_eq "apply migrate digest" "$(grep -c 'ghcr.io/alexandrezanata/abastevo-migrate@sha256:' "$MIGRATE_YAML")" "1"
fi
if [[ -n "$BACKUP_YAML" ]]; then
    assert_eq "apply backup hostPath" "$(grep -c 'hostPath:' "$BACKUP_YAML")" "1"
fi
assert_eq "apply backup retention" "$(find "$CASE/backup" -maxdepth 1 -name 'pre-*.dump' | wc -l)" "7"
assert_absent "apply does not call other tools" "$CASE/mutation"

echo "== apply: contract floor refusal =="
CASE="$(new_case apply-contract)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
jq '.schema_min_supported = "000133" | .migrations_max = "000133"' "$CASE/artifact/release-manifest.json" > "$CASE/manifest.tmp"
mv "$CASE/manifest.tmp" "$CASE/artifact/release-manifest.json"
write_oci "$CASE"
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
expect_failure "apply contract floor refused" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    pass "contract failure receipt exists"
    assert_eq "contract failure stage" "$(jq -r '.stage' "$RECEIPT")" "schema"
    assert_eq "contract failure result" "$(jq -r '.result' "$RECEIPT")" "failed"
else
    failed "contract failure receipt exists"
fi
assert_eq "contract current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_no_job_role "contract no backup job" "$CASE" backup
assert_no_job_role "contract no migrate job" "$CASE" migrate
if [[ -f "$CASE/kubectl.log" ]]; then
    assert_eq "contract no rollout undo" "$(grep -c 'rollout undo' "$CASE/kubectl.log" || true)" "0"
else
    pass "contract no rollout undo"
fi
assert_eq "contract landing unchanged" "$(readlink "$CASE/landing/current")" "releases/$FIXTURE_OLD_SHA"

echo "== apply: bootstrap without a current floor is refused =="
CASE="$(new_case apply-bootstrap)"
prepare_landing "$CASE" 1
KUBECTL_SCHEMA_ANNOTATION=000120 expect_failure "apply bootstrap refused" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "bootstrap failure stage" "$(jq -r '.stage' "$RECEIPT")" "schema"
fi
assert_no_job_role "bootstrap no backup job" "$CASE" backup
assert_no_job_role "bootstrap no migrate job" "$CASE" migrate

echo "== apply: backup failure stops before migration =="
CASE="$(new_case apply-backup-fail)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
KUBECTL_FAIL_ROLE=backup expect_failure "apply backup job failure" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "backup failure stage" "$(jq -r '.stage' "$RECEIPT")" "backup"
    assert_eq "backup failure no undo" "$(jq -r '.rollout_undone' "$RECEIPT")" "false"
fi
assert_eq "backup failure current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_no_job_role "backup failure no migrate job" "$CASE" migrate
assert_eq "backup failure no rollout" "$(grep -c 'set image' "$CASE/kubectl.log" || true)" "0"

echo "== apply: backup checksum mismatch stops before migration =="
CASE="$(new_case apply-backup-hash)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
KUBECTL_BACKUP_BAD_HASH=1 expect_failure "apply backup checksum mismatch" run_agent_apply "$CASE"
assert_no_job_role "backup hash no migrate job" "$CASE" migrate
assert_eq "backup hash current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"

echo "== apply: migration failure does not start the rollout =="
CASE="$(new_case apply-migrate-fail)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
KUBECTL_FAIL_ROLE=migrate expect_failure "apply migration job failure" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "migrate failure stage" "$(jq -r '.stage' "$RECEIPT")" "migrate"
    assert_eq "migrate failure no undo" "$(jq -r '.rollout_undone' "$RECEIPT")" "false"
fi
assert_eq "migrate failure current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_eq "migrate failure no set image" "$(grep -c 'set image' "$CASE/kubectl.log" || true)" "0"

echo "== apply: broken rollout is undone =="
CASE="$(new_case apply-rollout-fail)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
KUBECTL_ROLLOUT_FAIL=1 expect_failure "apply broken rollout undone" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "rollout failure stage" "$(jq -r '.stage' "$RECEIPT")" "rollout"
    assert_eq "rollout failure undone" "$(jq -r '.rollout_undone' "$RECEIPT")" "true"
fi
assert_eq "rollout failure undo command" "$(grep -c 'rollout undo deployment/abastevo-temp-api' "$CASE/kubectl.log" || true)" "1"
assert_eq "rollout failure current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_eq "rollout failure landing unchanged" "$(readlink "$CASE/landing/current")" "releases/$FIXTURE_OLD_SHA"
assert_absent "rollout failure no new landing" "$CASE/landing/releases/$FIXTURE_SHA"

echo "== apply: smoke failure keeps the previous landing and undoes images =="
CASE="$(new_case apply-smoke-fail)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
SMOKE_MODE=fail expect_failure "apply smoke failure undone" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "smoke failure stage" "$(jq -r '.stage' "$RECEIPT")" "smoke"
    assert_eq "smoke failure undone" "$(jq -r '.rollout_undone' "$RECEIPT")" "true"
    assert_eq "smoke failure landing not switched" "$(jq -r '.landing_switched' "$RECEIPT")" "false"
fi
assert_eq "smoke failure undo command" "$(grep -c 'rollout undo deployment/abastevo-temp-api' "$CASE/kubectl.log" || true)" "1"
assert_eq "smoke failure current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_eq "smoke failure landing unchanged" "$(readlink "$CASE/landing/current")" "releases/$FIXTURE_OLD_SHA"
assert_absent "smoke failure no new landing" "$CASE/landing/releases/$FIXTURE_SHA"

echo "== apply: concurrent lock is refused =="
CASE="$(new_case apply-lock)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
CURRENT_BEFORE="$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)"
flock "$CASE/state/apply.lock" -c 'sleep 2' &
LOCK_PID=$!
sleep 0.3
expect_failure "apply concurrent lock refused" run_agent_apply "$CASE"
wait "$LOCK_PID" 2>/dev/null || true
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'failure-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "lock failure stage" "$(jq -r '.stage' "$RECEIPT")" "lock"
fi
assert_eq "lock current unchanged" "$(sha256sum "$CASE/state/current.json" | cut -d' ' -f1)" "$CURRENT_BEFORE"
assert_absent "lock did not call kubectl" "$CASE/kubectl.log"

echo "== apply: surge requires quota headroom =="
CASE="$(new_case apply-no-headroom)"
make_current_old "$CASE"
prepare_landing "$CASE" 1
ABASTEVO_ALLOW_SURGE=1 KUBECTL_QUOTA_USED=6000m expect_success "apply no-headroom success" run_agent_apply "$CASE"
RECEIPT="$(find "$CASE/receipts" -maxdepth 1 -name 'deploy-*.json' -type f | sort | tail -n 1)"
if [[ -n "$RECEIPT" ]]; then
    assert_eq "no-headroom surge false" "$(jq -r '.surge' "$RECEIPT")" "false"
    assert_eq "no-headroom maxSurge" "$(jq -r '.strategy.maxSurge' "$RECEIPT")" "0"
    assert_eq "no-headroom maxUnavailable" "$(jq -r '.strategy.maxUnavailable' "$RECEIPT")" "1"
    assert_eq "no-headroom gap recorded" "$(jq -r '.strategy.gap' "$RECEIPT")" "true"
    assert_eq "no-headroom headroom" "$(jq -r '.quota_headroom_milli' "$RECEIPT")" "0"
fi
assert_eq "no-headroom strategy patch" "$(grep -c 'maxSurge' "$CASE/kubectl.log" || true)" "0"

echo "== summary: $PASS passed, $FAIL failed =="
[[ "$FAIL" -eq 0 ]]
