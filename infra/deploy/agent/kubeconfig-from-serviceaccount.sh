#!/usr/bin/env bash
# Build a namespace-scoped kubeconfig from a ServiceAccount token.
#
# The token is read from kubectl's stdout through stdin; it is never printed,
# never passed as a command-line argument and never written outside the
# operator-owned output file (mode 600). Run this only after the reviewed RBAC
# objects are applied.
set -euo pipefail

namespace="${ABASTEVO_NAMESPACE:?ABASTEVO_NAMESPACE is required}"
service_account="${ABASTEVO_SERVICE_ACCOUNT:?ABASTEVO_SERVICE_ACCOUNT is required}"
output="${ABASTEVO_KUBECONFIG_OUT:?ABASTEVO_KUBECONFIG_OUT is required}"
duration="${ABASTEVO_TOKEN_DURATION:-8760h}"

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 1; }

server="$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.server}')"
ca="$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')"
[[ -n "$server" ]] || { echo "current kubeconfig has no cluster server" >&2; exit 1; }
[[ -n "$ca" ]] || { echo "current kubeconfig has no certificate authority data" >&2; exit 1; }

install -d -m 0700 "$(dirname "$output")"
umask 077
tmp="$(mktemp "${output}.XXXXXX")"
script="$(mktemp)"
cleanup() { rm -f "$tmp" "$script"; }
trap cleanup EXIT

cat > "$script" <<'PY'
import sys
import yaml

path, server, ca, namespace, name = sys.argv[1:6]
token = sys.stdin.read().strip()
if not token:
    raise SystemExit("kubectl returned an empty token")

config = {
    "apiVersion": "v1",
    "kind": "Config",
    "clusters": [{
        "name": "staging",
        "cluster": {
            "server": server,
            "certificate-authority-data": ca,
        },
    }],
    "contexts": [{
        "name": name,
        "context": {
            "cluster": "staging",
            "namespace": namespace,
            "user": name,
        },
    }],
    "current-context": name,
    "users": [{
        "name": name,
        "user": {"token": token},
    }],
}
with open(path, "w", encoding="utf-8") as handle:
    yaml.safe_dump(config, handle, sort_keys=False)
PY

kubectl -n "$namespace" create token "$service_account" --duration="$duration" \
    | python3 "$script" "$tmp" "$server" "$ca" "$namespace" "$service_account"

chmod 0600 "$tmp"
mv "$tmp" "$output"
trap - EXIT
echo "wrote $output (mode 600); keep it outside Git"
