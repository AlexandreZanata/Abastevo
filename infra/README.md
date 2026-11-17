# Infrastructure

Local services, container images, edge configuration, staging manifests and
the operational scripts for the backend and the static web bundle. Nothing
here holds secrets, provider credentials, database backups or evidence
objects: real values live on the hosts, outside Git.

Staging runs on VPS 72.60.147.2 at https://abastevo.com.br, in k3s namespace
`abastevo-temp`, and is updated only by the signed pull agent in
`deploy/agent/`. Production is not provisioned.

## Layout

| Path | Contents |
|------|----------|
| `compose.dev.yml` | Local PostGIS on `127.0.0.1:5434` (volume `anpfuel-pgdata-dev`, dev-only credentials). |
| `compose.validation.yml` | Isolated tmpfs PostGIS + private S3 (`storage-init`) for `scripts/tests/test-local-backend.sh`; unique project, dynamic loopback ports, synthetic credentials. |
| `compose.staging.yml`, `compose.prod.yml`, `env.*.example`, `caddy/Caddyfile` | Compose topology (db, one-shot migrate, api, worker, Caddy) gated by `make check-infra`. Examples carry `REPLACE_ME` placeholders only. |
| `caddy/staging-web.Caddyfile` | Web edge for the signed landing bundle, portal, `/admin/` and `/empresas/`; mirrored byte-for-byte by `k8s/staging/web-caddy-configmap.yaml`. |
| `caddy/portal-*.Caddyfile` | Loopback-only portal rehearsal and preview edges. |
| `docker/Dockerfile` | Targets `api` (with `/ops`), `worker` (with Rust preparers), `migrate`, `station-load`; static binaries on distroless, user 65532. |
| `db/runtime-role.sql` | Generated runtime-role grant inventory (see `backend/README.md`). |
| `k8s/staging/` | Staging manifests, render and diff tooling ([README](k8s/staging/README.md)). |
| `k8s/registry-sync-cronjob.yaml` | Registry-sync CronJob template (`__REVISION__` placeholder). |
| `deploy/agent/` | Signed pull deploy agent and its systemd units ([README](deploy/agent/README.md)). |
| `deploy/registry/` | Daily ANP / monthly OpenCNPJ refresh units ([README](deploy/registry/README.md)). |
| `charging/` | EV catalog review timers ([README](charging/README.md)). |
| `scripts/` | `deploy.sh`, `backup.sh`, `restore.sh`, `rehearse.sh`, `load/` (load, faults, portal capacity), portal rehearsal/preview/smoke, `rollout-probe.py`. |

## Commands

From the repository root:

```sh
docker compose -f infra/compose.dev.yml up -d db     # local PostGIS
docker compose -f infra/compose.dev.yml stop db      # keeps the volume
docker compose -f infra/compose.dev.yml down -v      # local reset only
docker build -f infra/docker/Dockerfile --target api --build-arg REVISION=$(git rev-parse --short HEAD) .
```

| Target | What it checks |
|--------|----------------|
| `make check-infra` / `make test-infra` | Staging manifest structure/render and Compose topology gate / its failure harness. |
| `make test-deploy` | `infra/scripts/deploy.sh` failure paths with stubbed docker/curl. |
| `make test-deploy-agent` | Deploy agent planner/apply/status harness (no network). |
| `make test-backup` / `make test-restore` | Encrypted backup pipeline / isolated restore drill on a disposable DB. |
| `make test-load` | Bounded load smoke plus DB/storage/disk fault suite on a disposable DB. |
| `make check-security` / `make test-security` | Pins, non-root runtimes, private DB, edge rules, secret scans, govulncheck / refusal harness. |
| `make test-rehearse` | Local release rehearsal on a disposable DB. |
| `make rehearse-portal`, `make preview-portal`, `make portal-capacity` | Loopback-only portal promotion, HTTPS preview/accessibility and capacity drills. |
| `make check-portal-capacity`, `make check-preview-probe`, `make check-publication-gate` | Self-tests of those harnesses. |
| `make preflight-portal-publication`, `make smoke-portal-public` | Portal publication preflight and read-only public smoke (env-driven; see `make help`). |

`bash scripts/infra-smoke.sh` is the static edge smoke (`--live <host>` at
deploy time); `BASE_URL=... bash scripts/cache-regression.sh` checks edge cache
behavior against a live origin.

## Configuration

- Compose: `ANPFUEL_ENV_FILE` (env file outside the repo), `ANPFUEL_RELEASE`
  (immutable release tag), `CADDY_DOMAIN`; backend variables are listed in
  `backend/README.md`.
- Staging manifests: placeholders rendered from an operator-owned env file
  (`STAGING_RENDER_ENV`); see `k8s/staging/README.md`.
- Deploy agent: `/etc/abastevo-deploy/agent.env` (`ABASTEVO_*` keys).

## Rules

- Never commit secrets, real env files, kubeconfigs, backups or receipts.
- Pin every image by digest or immutable release tag, never `latest`.
- Only the edge publishes ports. The database, metrics (`ANPFUEL_METRICS_ADDR`
  stays loopback) and admin endpoints are never public.
- `ANPFUEL_CANONICAL_HOST` equals the public edge host: request signatures
  cover it and proxy hosts are untrusted.
- Caddy trusts only private-range proxies, drops request/response headers from
  access logs (signature headers, contributor IPs and nearby queries are
  private) and strips `Cookie`/`Authorization` on shared-cacheable GETs. The
  API marks writes, owner reads, nearby reads and errors `no-store`.
- Migrate runs as a one-shot job before the rollout, never via `up`. There are
  no down migrations: rollback is the previous release image, and
  `down -v` is never run against a persistent volume.
- Keep `caddy/staging-web.Caddyfile` and its ConfigMap identical
  (`python3 business-web/tests/caddy.py` checks both, needs Docker).
- On staging the agent changes only images and the landing release; Secret,
  ConfigMap and env changes are explicit operator steps.

## Further reading

[Operations runbooks](../docs/operations/) · [Backup](../docs/operations/backup.md) ·
[Recovery](../docs/operations/recovery.md) ·
[Monitoring](../docs/operations/monitoring.md) ·
[Registry daily sync](../docs/operations/registry-daily-sync.md) ·
[Security and privacy](../docs/security/security-privacy.md)
