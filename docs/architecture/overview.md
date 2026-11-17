# System overview

Abastevo has one backend, one database and several clients. Everything that
users see is either an official record (ANP surveys, the station registry,
fuel-quality and enforcement sources) or a community record, and the two are
never merged into one value.

## Components

| Component | Code | Role |
|-----------|------|------|
| Android app | `app`, `application`, `domain`, `data`, `maps/engine` | Browse official prices offline, map and routing, vehicles, community and forum, enterprise driver features. |
| API | `backend/cmd/api` | Public anonymous reads, signed writes, private owner reads, admin and enterprise endpoints. |
| Worker | `backend/cmd/worker` | Durable PostgreSQL job queue: ingestion, projections, notifications, media cleanup, portal generation. |
| Batch tools | `backend/cmd/station-*`, `map-*`, `portal*`, `charging-catalog`, `migrate`, `ops` | Station registry sync and load, map packages, static portal build/verify/release, migrations, operator commands. |
| Source preparers | `tools/station-prep` (Rust) | Turn official files into verified, hashed batches the Go loaders publish. |
| Database | PostgreSQL + PostGIS | Single source of truth; migrations in `backend/db/migrations`. |
| Media | Private S3-compatible storage | Photo evidence and enterprise originals, presigned and short-lived. |
| Landing and portal | `landing`, `backend/internal/modules/enterpriseexport/portalgen` | Static site and SEO portal (`/precos`, `/postos`, `/forum`, sitemaps) at abastevo.com.br. |
| Admin web | `admin-web` | Staff console (moderation, tiers, contributors). |
| Business web | `business-web` | Abastevo Empresas web app for companies. |

## Main flows

- **Official prices:** the worker downloads ANP survey files, publishes
  immutable revisions and exposes them through `/v1`; the app also caches
  survey weeks for offline use.
- **Station registry:** a daily job prepares the national registry with the Rust
  preparer and loads it through `station-sync`/`station-load`
  ([registry daily sync](../operations/registry-daily-sync.md)).
- **Community:** the app signs writes with a device key (proof of possession);
  submissions go through validation, optional photo evidence in private storage
  and conservative consensus before they affect a projection.
- **Portal:** the worker generates the static portal from published data and
  the edge serves it with sitemaps; indexing rules live in
  [enterprise/portal-routes.md](../enterprise/portal-routes.md).
- **Enterprise:** organization-scoped data behind row-level security and
  explicit permissions ([enterprise architecture](../enterprise/architecture.md)).

## Deployment

Staging is the live environment: a VPS running k3s (namespace `abastevo-temp`)
behind host Caddy and Cloudflare, serving `abastevo.com.br`, `api.` and
`admin.`. A merge to `main` triggers CI; when all workflows pass,
`release-staging` publishes a signed manifest with image digests and the
landing bundle. The deploy agent on the VPS verifies the signature, backs up,
migrates, rolls out and smoke-tests. Android builds are signed by
`android-release` and published as GitHub prereleases. See
[operations](../operations/).

## Where to read next

- [Backend architecture](backend.md): modules, transactions, jobs, caching.
- [Backend domain model](../backend/domain-model.md) and [data model](../backend/data-model.md).
- [Android design system](../android/design-system.md) and [map engine](../android/map-engine.md).
- [Product rules](../product/) and [decisions](../decisions/).
