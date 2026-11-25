# Staging deploy agent

Pull-based deployer for staging. A systemd timer on the staging host (VPS
72.60.147.2, k3s namespace `abastevo-temp`, https://abastevo.com.br) runs it
every four minutes; it is the only path by which a release reaches staging.
It deploys only what `release-staging` signed after every `main` workflow
passed. Production is out of scope.

## What a run does

Verification and plan (every mode):

1. Fetch the signed `abastevo-release:staging` OCI artifact (or `sha-<commit>`).
2. Verify the manifest digest and the Sigstore keyless signature against the
   exact `release-staging.yml@refs/heads/main` identity and GitHub OIDC issuer.
3. Validate the `abastevo-release-manifest-v1` schema; check through the
   anonymous GitHub compare API that the commit is reachable from `main` and
   descends from the deployed commit. Positive answers are remembered in
   `github-verified.json`, so a run for an already verified commit makes no
   GitHub request (see [GitHub API quota](#github-api-quota)).
4. Compare with `current.json` and write a mode-600 plan receipt
   (`abastevo-deploy-plan-v1`; `action` is `no-change` when nothing differs).

Apply (`ABASTEVO_APPLY=1`), in fixed order:

1. Schema floor gate: refuse without a baseline, when the running floor is
   above the database, or when the target `schema_min_supported` is above the
   current floor (a contract change is never automatic).
2. Verified backup Job (`pg_dump -Fc`, `pg_restore --list`, SHA-256 re-checked
   on the host; last 7 kept).
3. Migration Job with the signed `migrate` image (`backoffLimit: 0`).
4. One `kubectl set image` for the `api` and `worker` containers, then
   `rollout status`.
5. Smoke: `/health/ready`, `/`, `/consultar/`, one station read, the portal
   manifest and one search shard.
6. Extract the signed landing layer to `releases/<sha>` and switch the relative
   `current` symlink atomically (last 5 kept); smoke again.
7. Write `current.json` last, then a `abastevo-deploy-receipt-v1` receipt.

Any failure after the image update runs `rollout undo` and restores the
previous landing; earlier failures leave the cluster untouched. Failed applies
write a failure receipt, and `current.json` changes only on success.
Migrations are never rolled back; the additive window keeps the previous image
valid.

## Commands

```sh
abastevo-deploy-agent                          # plan (and apply when ABASTEVO_APPLY=1)
abastevo-deploy-agent plan --recheck <sha>     # retry a latched failed commit after fixing it
abastevo-deploy-agent baseline --sha <sha> [--floor <NNNNNN>]   # record existing state once
abastevo-deploy-agent release-hold             # clear an operator hold after restoring service
bash /usr/local/lib/abastevo-deploy-agent/status.sh   # read-only status
```

- `baseline` reads the database maximum, the running images, `/health/ready`
  and the landing digest, and writes `current.json` with `baseline: true`. The
  floor comes from the release manifest or `--floor`, never from the database.
- State files in `ABASTEVO_STATE_DIR`: `current.json`, `checked.json` (failed
  commits are latched and skipped), `operator-hold.json` (written after an
  incomplete rollback; all runs refuse until `release-hold`), `inflight.json`
  (an interrupted candidate is converged before newer releases),
  `github-verified.json` (the last 32 positive GitHub ancestry answers; delete
  it to force fresh checks).
- All commands share one non-blocking lock; a busy run prints `Deferred:` and
  exits 0.
- Pause and resume are explicit owner decisions:
  `systemctl disable --now abastevo-deploy-agent.timer` /
  `systemctl enable --now abastevo-deploy-agent.timer`. Disabling the timer does
  not cancel a running apply.

## GitHub API quota

The agent uses GitHub's anonymous REST API, whose limit (60 requests/hour) is
per IP address and shared with every other project on the host. Conditional
requests answered with `304` still count when anonymous. The staging host
splits the quota into 15 requests/hour per project, so the timer runs every
four minutes. Ancestry is immutable on the protected `main`, so only positive
answers are cached: a no-change or latched run spends nothing, and a new
release spends one or two requests once. Refusals and HTTP errors are never
cached. An exhausted quota shows as `GitHub reachability check failed with
HTTP 403` in the journal and clears at the hourly reset.

## Host configuration

Only `/etc/abastevo-deploy/agent.env` (root, mode 600, never committed) and the
kubeconfig it names. Keys:

- Required: `ABASTEVO_NAMESPACE`, `ABASTEVO_API_DEPLOYMENT`,
  `ABASTEVO_WORKER_DEPLOYMENT` (may equal the API deployment),
  `ABASTEVO_MIGRATE_JOB`, `ABASTEVO_LANDING_ROOT`, `ABASTEVO_BACKUP_DIR`,
  `ABASTEVO_RECEIPT_DIR`.
- Apply: `ABASTEVO_APPLY`, `ABASTEVO_SMOKE_BASE_URL`, `KUBECONFIG`.
- Optional: `ABASTEVO_STATE_DIR`, `ABASTEVO_GITHUB_REPOSITORY`,
  `ABASTEVO_GITHUB_API`, `ABASTEVO_WORKFLOW_PATH`,
  `ABASTEVO_MANIFEST_REPOSITORY`, `ABASTEVO_MANIFEST_TAG`,
  `ABASTEVO_API_CONTAINER`, `ABASTEVO_WORKER_CONTAINER`, `ABASTEVO_DB_SECRET`,
  `ABASTEVO_DB_IMAGE`, `ABASTEVO_ROLLOUT_TIMEOUT`, `ABASTEVO_JOB_TIMEOUT`,
  `ABASTEVO_SMOKE_TIMEOUT`, `ABASTEVO_BACKUP_KEEP`, `ABASTEVO_LANDING_KEEP`,
  `ABASTEVO_ALLOW_SURGE`, `ABASTEVO_SURGE_RESOURCE_QUOTA`,
  `ABASTEVO_SURGE_CPU_MILLI`, `ABASTEVO_KUBECTL`, `ABASTEVO_LIBDIR`,
  `ABASTEVO_LOCK_FILE`, `ABASTEVO_FAILURE_HOOK`.

Defaults are at the top of `abastevo-deploy-agent`. Surge is off by default;
`ABASTEVO_ALLOW_SURGE=1` also requires independent storage and quota headroom
for one extra pod and refuses rather than falling back. `ABASTEVO_FAILURE_HOOK`
names a root/service-owned executable that receives only result, SHA and stage
on stdin (10 s bound). No GHCR credential or GitHub token is used.

Host tools: `cosign` v3.1.x, `oras` v1.3.x, `curl`, `jq`, `flock`, `tar`,
`sha256sum`, Python 3, and `kubectl` for apply.

## Install and RBAC

Files: `abastevo-deploy-agent` → `/usr/local/bin/`; `apply.sh`, `baseline.sh`,
`safety.sh`, `status.sh`, `surge-capacity.py`, `indexnow.py` and `runtime/k8s.sh` →
`/usr/local/lib/abastevo-deploy-agent/`. Install `abastevo-deploy-agent.service`,
`abastevo-deploy-agent.timer`, `abastevo-indexnow.service` and
`abastevo-indexnow.timer` from `systemd/` into `/etc/systemd/system/`.
The service runs as the `abastevo-deploy` system user with
`ProtectSystem=strict`, `NoNewPrivileges=yes` and a 20-minute bound; a drop-in
adds `ReadWritePaths=` for the landing and backup roots. Upgrade an active host
by staging a complete versioned bundle and switching it under the service lock;
never install partial libraries while a run can source them.

`infra/k8s/staging/rbac/deploy-agent-rbac.yaml` grants the agent's
ServiceAccount only: Deployments get/list/watch/patch, ReplicaSets read, Jobs
create/get/list/watch/delete, Pods and pod logs read, ResourceQuotas read. No
Secrets, ConfigMaps, Services, exec or cluster scope. Create the kubeconfig
without printing the token:

```sh
ABASTEVO_NAMESPACE=abastevo-temp ABASTEVO_SERVICE_ACCOUNT=abastevo-deploy-agent \
ABASTEVO_KUBECONFIG_OUT=/etc/abastevo-deploy/kubeconfig \
bash infra/deploy/agent/kubeconfig-from-serviceaccount.sh
```

The optional IndexNow timer submits changes from the public sitemaps every
30 minutes. It is off by default and runs independently of deploys. The owner
installs `indexnow.py` and both IndexNow units at the paths above, then sets
`ABASTEVO_INDEXNOW_KEY` and `ABASTEVO_INDEXNOW_ENABLED=1` in `agent.env` once the key
file is served. Reload systemd and enable the timer with
`systemctl enable --now abastevo-indexnow.timer`. Set `ABASTEVO_INDEXNOW_ENABLED=0`
to disable requests. The [IndexNow runbook](../../../docs/operations/deploy-agent.md#indexnow-submitter)
lists all environment variables and the timer commands.

## Tests

```sh
make test-deploy-agent      # agent, status, surge capacity, IndexNow and rollout-probe harnesses
cd backend && go test -tags=integration -count=1 ./cmd/worker -run TestOverlappingWorkersDoNotDuplicateEffects
```

The IndexNow tests use a loopback HTTP server. The deploy harness stubs `oras`, `cosign`, `curl`, `kubectl`, `docker`, `helm` and
`systemctl`, makes no network request and writes only to a temporary
directory. The Go test (disposable PostGIS) proves two overlapping workers
never duplicate a job effect.

## Rules

- The agent changes only images, the landing release and its own state.
  Secrets, ConfigMaps, env and manifests are explicit operator changes
  ([k8s/staging](../../k8s/staging/README.md)).
- Never edit `current.json`, receipts or the journal by hand; never baseline or
  recheck over an unfinished `inflight.json`.
- Never put tokens, SSH keys or provider credentials in the host config.

See [operations runbooks](../../../docs/operations/) and
[recovery](../../../docs/operations/recovery.md).
