# Abastevo staging deploy agent (Phase 2, dry run)

Status: LOCAL_DONE / INTEGRATION_PENDING. This agent plans a release; it does
not deploy, migrate, switch landing content or change any deployment state.

The agent runs from a systemd timer on the staging host. Each run:

1. fetches the signed `staging` OCI artifact from
   `ghcr.io/<owner>/abastevo-release`;
2. verifies the OCI manifest structure, the manifest layer digest and the
   Sigstore keyless signature with the exact workflow identity and GitHub OIDC
   issuer;
3. validates the `abastevo-release-manifest-v1` schema;
4. checks that the release commit is an ancestor of `main` through the public
   GitHub API (anonymous, no token);
5. compares the target with `/var/lib/abastevo-deploy/current.json`;
6. writes a `abastevo-deploy-plan-v1` receipt with the proposed changes.

It never writes `current.json` and never calls `docker`, `kubectl`, `helm` or
`systemctl`. The optional `schema_min_supported` field is validated when a
manifest carries it; the signed Phase 1 manifests without the field remain
readable. Phase 3 requires it for apply.

The signed OCI artifact also carries `landing-dist.tar.gz` as an
`application/vnd.abastevo.landing.tar+gzip` layer. Its SHA-256 is the
`landing_sha256` in the signed manifest; the Phase 3 executor extracts that
layer and switches the relative `current` symlink.

## Prerequisites

The host must provide these pinned tools on `PATH` (installed outside the
repository, never by a `curl | bash` step):

- `cosign` v3.1.x (tested with v3.1.4)
- `oras` v1.3.x (tested with v1.3.4)
- `curl`, `jq`, `sha256sum`, `mktemp`, `install`, `date`

No GHCR credential is needed: the release packages are public. No GitHub token
is needed: the reachability check uses the anonymous compare API. Do not put a
token, SSH key or provider credential in the host config.

## Host configuration

The only host configuration is `/etc/abastevo-deploy/agent.env`, owned by root
and readable only by root (`0600`). It is never committed. Required keys:

```sh
ABASTEVO_NAMESPACE=<namespace-or-compose-project>
ABASTEVO_API_DEPLOYMENT=<api-service-or-deployment>
ABASTEVO_WORKER_DEPLOYMENT=<worker-service-or-deployment>
ABASTEVO_MIGRATE_JOB=<migrate-job-or-service>
ABASTEVO_LANDING_ROOT=<absolute-landing-release-root>
ABASTEVO_BACKUP_DIR=<absolute-backup-root>
ABASTEVO_RECEIPT_DIR=<absolute-writable-receipt-directory>
```

Optional keys (defaults shown):

```sh
ABASTEVO_STATE_DIR=/var/lib/abastevo-deploy
ABASTEVO_GITHUB_REPOSITORY=AlexandreZanata/Abastevo
ABASTEVO_GITHUB_API=https://api.github.com
ABASTEVO_WORKFLOW_PATH=.github/workflows/release-staging.yml
ABASTEVO_MANIFEST_REPOSITORY=ghcr.io/alexandrezanata/abastevo-release
ABASTEVO_MANIFEST_TAG=staging
KUBECONFIG=/etc/abastevo-deploy/kubeconfig
```

`ABASTEVO_RECEIPT_DIR` and `ABASTEVO_STATE_DIR` must be under a directory the
service can write. The supplied unit uses `/var/lib/abastevo-deploy`; keep the
defaults or add a systemd drop-in with the matching `ReadWritePaths`.

## Install

```sh
install -m 0755 infra/deploy/agent/abastevo-deploy-agent /usr/local/bin/abastevo-deploy-agent
install -m 0644 infra/deploy/agent/systemd/abastevo-deploy-agent.service /etc/systemd/system/
install -m 0644 infra/deploy/agent/systemd/abastevo-deploy-agent.timer /etc/systemd/system/
useradd --system --home /var/lib/abastevo-deploy --shell /usr/sbin/nologin abastevo-deploy
install -d -m 0700 -o abastevo-deploy -g abastevo-deploy /var/lib/abastevo-deploy
install -d -m 0700 /etc/abastevo-deploy
install -m 0600 -o root -g root /dev/null /etc/abastevo-deploy/agent.env
# Fill the required keys with the host editor; never commit or log the file.
systemctl daemon-reload
systemctl enable --now abastevo-deploy-agent.timer
```

Run one planning pass and inspect the journal and receipt:

```sh
systemctl start abastevo-deploy-agent.service
journalctl -u abastevo-deploy-agent.service -n 50 --no-pager
ls -l /var/lib/abastevo-deploy/receipts/
```

The timer runs every two minutes. The service uses `flock` so overlapping runs
are skipped. `ProtectSystem=strict`, `NoNewPrivileges=yes`, `PrivateTmp=yes`
and a dedicated user limit the blast radius.

## RBAC and kubeconfig (R8)

`infra/k8s/staging/deploy-agent-rbac.yaml` defines the namespace-scoped
ServiceAccount, Role and RoleBinding for `abastevo-temp`. It is review-only
here; do not apply it until the executor is approved. The Role grants only:

- Deployments: get/list/watch/patch (set image, rollout status, rollout undo)
- ReplicaSets: get/list/watch (rollout status)
- Jobs: create/get/list/watch/delete (backup and migration jobs)
- Pods: get/list/watch and pods/log get (job observation)
- ResourceQuotas: get/list (surge headroom)

It grants no secrets, ConfigMaps, Services, pods/exec, other namespaces or
cluster-scoped resources. The agent uses a kubeconfig outside the cluster; the
ServiceAccount token is not mounted into workload pods.

After the RBAC is applied, generate the kubeconfig without printing the token:

```sh
ABASTEVO_NAMESPACE=abastevo-temp \
ABASTEVO_SERVICE_ACCOUNT=abastevo-deploy-agent \
ABASTEVO_KUBECONFIG_OUT=/etc/abastevo-deploy/kubeconfig \
bash infra/deploy/agent/kubeconfig-from-serviceaccount.sh
```

The helper reads the token from `kubectl create token` through stdin, writes the
kubeconfig with mode 600 and never prints the token. The default token duration
is 8760h; rotate it before expiry with the same command. Keep both files outside
Git.

## Plan receipt

The agent writes `plan-<UTC timestamp>-<pid>.json` with mode `0600`:

```json
{
  "schema": "abastevo-deploy-plan-v1",
  "created_at": "2026-10-27T00:00:00Z",
  "action": "plan-only",
  "manifest": {
    "reference": "ghcr.io/<owner>/abastevo-release:staging",
    "oci_digest": "sha256:...",
    "file_digest": "sha256:...",
    "signature": "verified",
    "certificate_identity": "https://github.com/<owner>/<repo>/.github/workflows/release-staging.yml@refs/heads/main",
    "certificate_oidc_issuer": "https://token.actions.githubusercontent.com",
    "sha_reachable_from_main": true
  },
  "current": null,
  "target": {
    "sha": "...",
    "images": {"api": "...", "worker": "...", "migrate": "..."},
    "landing_sha256": "...",
    "migrations_max": "000132",
    "schema_min_supported": "000132"
  },
  "changes": {
    "release": true,
    "api": true,
    "worker": true,
    "migrate": true,
    "landing": true,
    "migrations": true
  },
  "host": {
    "namespace": "...",
    "api_deployment": "...",
    "worker_deployment": "...",
    "migrate_job": "..."
  }
}
```

`action` is `no-change` when every flag is false. A malformed or unsigned
manifest, a foreign certificate identity/issuer, a non-ancestor commit, a
GitHub API error, a registry error or an invalid `current.json` all exit
non-zero without writing a receipt.

## Current state file (written by Phase 3, read here)

`/var/lib/abastevo-deploy/current.json` must use
`abastevo-deploy-current-v1`:

```json
{
  "schema": "abastevo-deploy-current-v1",
  "sha": "...",
  "images": {"api": "...", "worker": "...", "migrate": "..."},
  "landing_sha256": "...",
  "migrations_max": "000132",
  "schema_min_supported": "000132",
  "applied_at": "..."
}
```

The Phase 2 agent only reads this file. Absence means "no current release" and
produces a `plan-only` receipt with `current: null`. Phase 3 writes it and uses
`schema_min_supported` for the expand/contract gate: a new release whose floor
is above the previous release floor contains a contract change and must not be
applied automatically while the previous image is still the rollback target.

## Tests

```sh
bash infra/deploy/agent/tests/test-agent.sh
```

The harness stubs `oras`, `cosign`, `curl`, `docker`, `kubectl`, `helm` and
`systemctl`, and proves: new-release planning, no-change planning, tampered and
unsigned manifests, foreign identity, non-ancestor/diverged commits, API and
registry failures, invalid schema, digest mismatch, missing signature layer,
invalid current state, missing configuration and absence of deployment calls.
It makes no network request and writes only under a temporary directory.

## Scope limits

Phase 2 is a dry run. Rollout, backup, migration, landing switch, smoke checks
and rollback are Phase 3. Production remains manual behind G09 and is out of
scope.
