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
