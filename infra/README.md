# Infrastructure workspace

Status: not provisioned. The [infrastructure plan](../docs/backend/INFRASTRUCTURE_PLAN.md) defines local, CI, staging and production, Compose/Caddy, backups, release/rollback, observability and load gates.

P01 adds local services and image builds. P08 adds deployment/operations artifacts and verifies them in controlled environments. G09 requires real operational evidence before Android improvement begins. Do not store provider credentials, evidence objects or database backups here.

## Local database (P01-T07)

Disposable PostgreSQL 18 + PostGIS 3.6 for development and integration tests:

```sh
docker compose -f infra/compose.dev.yml config   # validate; confirm 127.0.0.1 binding
docker compose -f infra/compose.dev.yml up -d db # start
docker exec anpfuel-dev-db-1 psql -U anpfuel -d anpfuel -c "SELECT PostGIS_version();"
docker compose -f infra/compose.dev.yml ps      # health
docker compose -f infra/compose.dev.yml stop db # stop (keeps volume)
```

- Image pinned by digest in `compose.dev.yml`; dev-only `anpfuel` credentials.
- Loopback only: host `127.0.0.1:5434` → container `5432` (workstation 5432/5433
  are taken by unrelated projects). Matches `backend/.env.example`.
- Data lives in the named local volume `anpfuel-pgdata-dev`, never a
  production volume. Explicit disposable-only reset:
  `docker compose -f infra/compose.dev.yml down -v` — never run an equivalent
  against persistent environments.

## Process images (P01-T10)

One pinned builder (`golang:1.27.1-bookworm` by digest) compiles the `api`,
`worker` and `migrate` roles; each target ships only its static binary on a
non-root distroless runtime with the same revision label (release provenance).
Secrets travel at runtime env, never in layers. The repo-root `.dockerignore`
keeps Android sources, docs and local env out of the build context.

```sh
docker build -f infra/docker/Dockerfile --target api \
  --build-arg REVISION=$(git rev-parse --short HEAD) .
docker run -d --name api -p 127.0.0.1:8080:8080 anpfuel-api:dev
curl http://127.0.0.1:8080/health/live  # 200
docker stop api && docker rm api        # clean terminate, exit 0
docker run --rm --network host -e ANPFUEL_DATABASE_URL=... anpfuel-migrate:dev
```

Verified: api/worker/migrate boot and terminate (exit 0); image Env carries
no secrets; `docker history` shows no credential layers; runtime user
`65532:65532`. Rollback: discard unpromoted images, return to the previous
digest; data and migrations are untouched by image changes.

## Staging and production topologies (P08-T01)

Review-only artifacts until provisioning is authorized: `compose.staging.yml`
(project `anpfuel-staging`) and `compose.prod.yml` (`anpfuel-prod`), sharing
`caddy/Caddyfile` plus per-environment templates (`env.staging.example`,
`env.prod.example`). Staging and production differ by project/network/volume
names, resource limits and credentials; the database image digest matches
development, and all other images pin digests or immutable release tags
(never `latest`).

Rules enforced by `make check-infra` (and its `make test-infra` mutant
harness): compose files parse; only Caddy publishes ports (80/443); no dev
credentials, loopback dev URLs or dev volumes; Caddy runs `admin off` with
TLS and `trusted_proxies private_ranges` only, and serves no metrics, admin
or database route; examples carry `REPLACE_ME` placeholders, never secrets.

```sh
cp infra/env.staging.example /srv/anpfuel/staging.env  # outside the repo
$EDITOR /srv/anpfuel/staging.env                        # fill real values
ANPFUEL_ENV_FILE=/srv/anpfuel/staging.env ANPFUEL_RELEASE=<tag> \
  docker compose -f infra/compose.staging.yml config   # validate
make check-infra                                        # topology gate
make test-infra                                         # gate failure proof
bash scripts/infra-smoke.sh                             # static smoke
bash scripts/infra-smoke.sh --live <host>               # edge smoke at deploy
```

Boot order per environment: `db` → `run --rm migrate` (one-shot, `tools`
profile, never via `up`) → `up -d api worker caddy`. Rollback returns to
the previous `ANPFUEL_RELEASE`; never `docker compose down -v` against
persistent volumes. `ANPFUEL_CANONICAL_HOST` must equal `CADDY_DOMAIN`
(signatures cover the canonical authority; proxy hosts are untrusted).

## Deploy and rollback (P08-T02)

`infra/scripts/deploy.sh` orchestrates one versioned release with
readiness/smoke gates and automatic return to the previous compatible
release; `make test-deploy` proves every failure path with stubbed
docker/curl. The full sequence, backup precheck, receipt keeping and
the CI publish design live in [the deploy runbook](../docs/operator/deploy.md).
No destructive down migration exists: rollback is a forward redeploy.

## Encrypted off-host backups (P08-T03)

`infra/scripts/backup.sh` writes daily encrypted dumps with a
roles/extensions manifest (`run`), checksum-verifies them including
the archive catalog (`verify`), prunes to 7 daily + 4 weekly copies
and alarms on stale backups (`check-age`); `make test-backup` proves
the pipeline against the disposable dev database. Schedules,
credential scope, transport and recovery linkage live in
[the backup runbook](../docs/operator/backup.md). The passphrase
stays outside the VPS; manifests carry no secrets.

## Isolated restore drill (P08-T04)
`infra/scripts/restore.sh` restores one verified artifact into a fresh
`anpfuel_drill_*` database, verifies the migration ledger, PostGIS,
per-table counts and evidence join integrity, and reports pending
ledger rows for `ops privacy replay` before traffic (see
[the recovery runbook](../docs/operator/recovery.md)); `make
test-restore` proves the full drill locally with the real ops binary.
Local drill evidence lives in
[docs/release-evidence/p08-t04-restore-drill.md](../docs/release-evidence/p08-t04-restore-drill.md);
staging acceptance repeats it on provisioned infrastructure.

## Metrics and alerts (P08-T05)

The API exposes Prometheus text on a loopback-only listener
(`ANPFUEL_METRICS_ADDR`, default `127.0.0.1:9090`, empty disables):
bounded request counters by method/route-template/class, pool gauges,
queue depth/dead-letter/oldest gauges and a dropped-series counter.
Topologies publish no metrics port. `ops monitoring eval` runs the
`db_down`/`jobs_dead`/`jobs_stuck`/`backup_stale` rules once for cron
integration (exit 1 when anything fires). Catalog, PromQL starters,
notification route and label policy live in
[the monitoring runbook](../docs/operator/monitoring.md).

## Edge cache (P08-T06)

Public station/price reads cache at the edge with bounded TTLs and
ETags; nearby, writes, owner reads and errors stay `no-store`, and
identity headers never change public bodies (proven by the
`*IgnoreIdentity` unit tests). Caddy strips `Cookie`/`Authorization`
on the shared-cacheable GET set. CDN allowlist, key policy, purge
procedure, signature-through-proxy notes and the live regression
suite (`BASE_URL=... bash scripts/cache-regression.sh`) live in
[the edge-cache runbook](../docs/operator/edge-cache.md).
