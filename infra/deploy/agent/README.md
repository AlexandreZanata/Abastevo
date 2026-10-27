# Abastevo staging deploy agent (dry run and k8s apply)

Status: dry-run and apply implementation LOCAL_DONE / INTEGRATION_PENDING.
`ABASTEVO_APPLY=0` (default) only plans. `ABASTEVO_APPLY=1` executes the
reviewed R3-R6 release order against namespace `abastevo-temp`. The RBAC is not
applied, the agent is not installed and the timer is not enabled in this
delivery; live staging acceptance is OWED.

The agent runs from a systemd timer on the staging host. In dry-run mode each
run:

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

In apply mode it keeps that verification and plan, then executes the schema
floor gate, verified backup, migration Job, one api/worker image rollout,
rollout status, smoke, landing switch, `current.json` last and a deployment
receipt. Any failure after the image update runs `rollout undo`; a switched
landing is restored; earlier failures leave the deployment untouched and write
a failure receipt. A dedicated `baseline` subcommand records the first real
state without deploying; apply refuses to run until a baseline exists.

The dry-run path never writes `current.json` and never calls `docker`,
`kubectl`, `helm` or `systemctl`. Apply mode calls `kubectl` only through
`runtime/k8s.sh`. The optional `schema_min_supported` field is validated when a
manifest carries it; apply requires it. The signed OCI artifact also carries
`landing-dist.tar.gz` as an `application/vnd.abastevo.landing.tar+gzip` layer
whose SHA-256 is the `landing_sha256` in the signed manifest; apply extracts
that layer and switches the relative `current` symlink.

## Prerequisites

The host must provide these pinned tools on `PATH` (installed outside the
repository, never by a `curl | bash` step):

- `cosign` v3.1.x (tested with v3.1.4)
- `oras` v1.3.x (tested with v1.3.4)
- `curl`, `jq`, `sha256sum`, `mktemp`, `install`, `date`
- apply mode: `kubectl` (or `ABASTEVO_KUBECTL`), `flock`, `tar`

No GHCR credential is needed: the release packages are public. No GitHub token
is needed: the reachability check uses the anonymous compare API. Do not put a
token, SSH key or provider credential in the host config. `pg_dump`,
`pg_restore` and the migration run inside Kubernetes Jobs; they are not host
dependencies.

## Host configuration

The only host configuration is `/etc/abastevo-deploy/agent.env`, owned by root
and readable only by root (`0600`). It is never committed. Required keys:

```sh
ABASTEVO_NAMESPACE=<k8s-namespace>
ABASTEVO_API_DEPLOYMENT=<api-deployment>
ABASTEVO_WORKER_DEPLOYMENT=<worker-deployment-or-same-as-api>
ABASTEVO_MIGRATE_JOB=<migrate-job-prefix>
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
ABASTEVO_API_CONTAINER=api
ABASTEVO_WORKER_CONTAINER=worker
ABASTEVO_SURGE_RESOURCE_QUOTA=abastevo-temp-quota
ABASTEVO_ALLOW_SURGE=0
ABASTEVO_SURGE_CPU_MILLI=2000
ABASTEVO_ROLLOUT_TIMEOUT=300
ABASTEVO_JOB_TIMEOUT=300
ABASTEVO_DB_SECRET=abastevo-temp-secrets
ABASTEVO_DB_IMAGE=postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677
ABASTEVO_BACKUP_KEEP=7
ABASTEVO_LANDING_KEEP=5
ABASTEVO_SMOKE_TIMEOUT=10
ABASTEVO_KUBECTL=kubectl
ABASTEVO_LIBDIR=/usr/local/lib/abastevo-deploy-agent
ABASTEVO_LOCK_FILE=/var/lib/abastevo-deploy/apply.lock
```

`ABASTEVO_WORKER_DEPLOYMENT` may equal `ABASTEVO_API_DEPLOYMENT`; apply then
updates both containers in one rollout.

Apply-only required keys:

```sh
ABASTEVO_APPLY=1
ABASTEVO_SMOKE_BASE_URL=<staging-https-origin>
KUBECONFIG=/etc/abastevo-deploy/kubeconfig
```

`ABASTEVO_RECEIPT_DIR` and `ABASTEVO_STATE_DIR` must be under a directory the
service can write. The supplied unit uses `/var/lib/abastevo-deploy`; apply
also needs write access to the landing and backup roots, supplied by a
systemd drop-in (see Install).

## Install

```sh
install -m 0755 infra/deploy/agent/abastevo-deploy-agent /usr/local/bin/abastevo-deploy-agent
install -d -m 0755 /usr/local/lib/abastevo-deploy-agent/runtime
install -m 0644 infra/deploy/agent/apply.sh /usr/local/lib/abastevo-deploy-agent/
install -m 0644 infra/deploy/agent/baseline.sh /usr/local/lib/abastevo-deploy-agent/
install -m 0644 infra/deploy/agent/runtime/k8s.sh /usr/local/lib/abastevo-deploy-agent/runtime/
install -m 0644 infra/deploy/agent/systemd/abastevo-deploy-agent.service /etc/systemd/system/
install -m 0644 infra/deploy/agent/systemd/abastevo-deploy-agent.timer /etc/systemd/system/
useradd --system --home /var/lib/abastevo-deploy --shell /usr/sbin/nologin abastevo-deploy
install -d -m 0700 -o abastevo-deploy -g abastevo-deploy /var/lib/abastevo-deploy
install -d -m 0700 /etc/abastevo-deploy
install -m 0600 -o root -g root /dev/null /etc/abastevo-deploy/agent.env
# Fill the required keys with the host editor; never commit or log the file.
install -d -m 0755 /etc/systemd/system/abastevo-deploy-agent.service.d
# Apply only: allow writes to the host landing and backup roots.
cat > /etc/systemd/system/abastevo-deploy-agent.service.d/paths.conf <<'EOF'
[Service]
ReadWritePaths=<landing-root> <backup-root>
EOF
systemctl daemon-reload
# Do not enable the timer until the reviewed staging acceptance completes.
```

Run one planning pass and inspect the journal and receipt:

```sh
systemctl start abastevo-deploy-agent.service
journalctl -u abastevo-deploy-agent.service -n 50 --no-pager
ls -l /var/lib/abastevo-deploy/receipts/
```

The timer runs every two minutes. The service uses `flock` so overlapping runs
are skipped; apply mode also takes an internal `apply.lock`. `ProtectSystem=strict`,
`NoNewPrivileges=yes`, `PrivateTmp=yes` and a dedicated user limit the blast
radius.

## Baseline

The first run against an existing staging host needs a reviewed baseline
instead of a synthetic current state:

```sh
ABASTEVO_APPLY=0 ... abastevo-deploy-agent baseline --sha <40-hex-main-commit> [--floor <NNNNNN>]
```

The command fetches the signed `abastevo-release:sha-<sha>` artifact (not the
moving `staging` tag), verifies the same keyless identity and that the commit
is an ancestor of `main`, reads the real database maximum with a schema Job,
reads the running `api` and `worker` images from the Deployment, verifies the
running deployment's `/health/ready` endpoint (the running image's
`migrate.Check` validates known migration checksums), and computes the current
landing tree digest. It writes `current.json` mode 600
with `baseline: true` and a `abastevo-deploy-baseline-v1` receipt, and performs
no rollout, migration or landing switch.

It refuses a non-ancestor SHA or a failing running schema-compatibility check.
The baseline uses `ABASTEVO_SMOKE_BASE_URL` for that readiness call.
`migrations_max` is the real database maximum. `schema_min_supported` is the
floor of the release, never the database maximum (a database-derived floor
would let contract changes up to that version pass as additive): it comes from
the release manifest, or from `--floor` when the release predates the field;
without either the baseline refuses, and a floor above the database refuses.
`release_match` records whether the running `api`/`worker` images are that
release's digests; `false` means the host runs hand-deployed images and the
first apply rolls the release images out.

## Apply mode (R3-R6)

Apply is opt-in (`ABASTEVO_APPLY=1`) and runs only after the same signed
manifest verification and plan. The order is fixed:

1. **Schema floor gate** - require `current.json`; without it apply refuses
   with `baseline required`. Read the current floor from that state, query the
   database with a short Job, and refuse when the running floor is above the
   database or the target floor is above the current floor. A higher target
   floor is a contract change and is never automatic. The live
   `abastevo.com/schema-version` annotation is informative only and is never a
   gate; the executor writes `abastevo.com/schema-floor`.
2. **Verified backup** - a Job runs `pg_dump -Fc`, `pg_restore --list` and
   writes the SHA-256 next to the archive. The agent re-hashes the archive on
   the host and keeps the last 7 `pre-*` pairs.
3. **Migration Job** - the signed `migrate` digest, `backoffLimit: 0`,
   `restartPolicy: Never`, `ttlSecondsAfterFinished: 86400`, DB secret via
   `envFrom`. The agent waits for completion and records the applied versions.
4. **Image rollout** - one `kubectl set image` updates the `api` and `worker`
   containers of the shared Deployment, then patches both pull policies to
   `IfNotPresent`; `kubectl rollout status --timeout` gates the result.
5. **Smoke** - `/health/ready`, `/`, `/consultar/`, one station-list read and
   the portal manifest plus one `dados/busca/` shard.
6. **Landing** - extract the signed landing layer to `releases/<sha>`, switch a
   relative `current` symlink atomically and keep the last 5 releases,
   including the previous rollback target. No pod restart is needed.
7. **State** - write `current.json` atomically last, then a
   `abastevo-deploy-receipt-v1` receipt.

Any failure after the image update runs `rollout undo` for the api and worker
containers. If the landing was already switched, the previous symlink is
restored. Failures before the image update write a failure receipt and leave
the deployment untouched. Migrations are never rolled back automatically; the
additive window keeps the previous image valid.

Surge is opt-in. `ABASTEVO_ALLOW_SURGE=1` is allowed only after the R5 worker
overlap proof and only when the ResourceQuota headroom is at least
`ABASTEVO_SURGE_CPU_MILLI`; otherwise the agent keeps
`maxSurge: 0` / `maxUnavailable: 1` and records the measured gap in the receipt.
The R5 overlap proof is still OWED, so the shipped default is no surge.

## RBAC and kubeconfig (R8)

`infra/k8s/staging/rbac/deploy-agent-rbac.yaml` defines the namespace-scoped
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

## Current state file (written by apply, read by dry run)

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

The dry-run agent only reads this file. Absence means "no current release";
apply refuses with `baseline required`. The `baseline` subcommand writes the
first file with `baseline: true`; normal applies write it last and replace it
atomically. `schema_min_supported` drives the expand/contract gate: a new
release whose floor is above the previous release floor contains a contract
change and must not be applied automatically while the previous image is still
the rollback target.

## Tests

```sh
bash infra/deploy/agent/tests/test-agent.sh
```

The harness stubs `oras`, `cosign`, `curl`, `kubectl`, `docker`, `helm` and
`systemctl`, and proves: new-release planning, no-change planning, tampered and
unsigned manifests, foreign identity, non-ancestor/diverged commits, API and
registry failures, invalid schema, digest mismatch, missing signature layer,
invalid current state, missing configuration and absence of deployment calls.
Apply cases cover success with surge and no-headroom gap, contract-floor
refusal, apply without `current.json`, backup Job failure, backup checksum
mismatch, migration failure, broken rollout undo, smoke failure with the
previous landing kept, concurrent lock refusal, `current.json` unchanged on
every failure, Job TTL/backoff, one shared set-image command, `schema-floor`
annotation, backup retention 7 and landing retention 5. Baseline cases cover
success, checksum drift and a SHA outside `main`. It makes no network request
and writes only under a temporary directory. Current count: 157 passed,
0 failed.

The R5 overlap proof is a separate PostGIS integration test:

```sh
cd backend
go test -tags=integration ./cmd/worker -run TestOverlappingWorkersDoNotDuplicateEffects -count=1
```

It runs two dispatcher loops with distinct worker IDs over one queue, asserts
both workers consumed work, every job has exactly one effect and no job has
`attempts > 1`.

## Scope limits

The dry-run and apply code is reviewed here. RBAC is not applied, the agent is
not installed, the timer is not enabled and no staging release is executed in
this delivery. Live acceptance (two consecutive merges, broken-image rollback,
contract refusal, landing switch and rollback) is OWED after review. The R5
overlap integration test passes for the queue dispatchers, but surge remains
disabled by default until the reviewed staging runs measure zero unavailable
replicas and the owner enables `ABASTEVO_ALLOW_SURGE=1`. Production remains
manual behind G09 and is out of scope.
