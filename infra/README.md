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
