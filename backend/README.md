# Backend

Go modular monolith behind the Abastevo Android app, the static landing/portal,
the admin panel (`admin-web/`) and the company web app (`business-web/`). It
owns the public `/v1` API, background jobs, the schema and the operator CLI;
not the Android code, the web builds or the Rust preparers
(`tools/station-prep/`), which it only invokes as trusted binaries. Stack: Go
`go1.27.4` (`go.mod` toolchain), net/http + chi, pgx, sqlc, slog, PostgreSQL 18
+ PostGIS 3.6, private S3-compatible storage.

## Layout

| Path | Contents |
|------|----------|
| `cmd/api` | Public HTTP process (`/v1`, `/health/live`, `/health/ready`). Never runs DDL. |
| `cmd/worker` | Background-job process over the PostgreSQL queue (dispatchers, schedules, source imports). |
| `cmd/migrate` | One-shot migrator: applies the embedded chain with a lock/checksum ledger. |
| `cmd/ops` | Restricted operator CLI (shipped as `/ops` in the api image); every command is audited. |
| `cmd/station-*` | Offline official-data tools: `station-sync` (daily registry refresh), `station-load`, `station-prices`, `station-locate`, `station-review`, `station-discover`. |
| `cmd/map*`, `cmd/portal*`, `cmd/charging-catalog` | Map overlay/distribution signing and verification, static portal generation/verification/promotion, EV catalog review. |
| `internal/modules/<module>/` | One bounded context each: `domain/` (pure rules), `application/` (use cases and ports), `adapters/` (sqlc/pgx stores, `http/` handlers, jobs). |
| `internal/platform/` | Shared infrastructure: `config`, `database`, `migrate`, `httpserver`/`httpapi`, `jobs`, `telemetry`, `health`, `apicontract` (OpenAPI and golden-vector tests), `runtimegrants`, and the enterprise plane (`orgapi`, `orgquota`, `orgtest`, `enterprisesuite`). |
| `db/migrations/` | Embedded, append-only SQL chain plus `schema_min_supported` (release compatibility floor). |
| `db/queries/<owner>/` | sqlc query files and the committed generated package for one owning module (`sqlc.yaml`). |
| `db/seeds/` | Idempotent dev/staging data; never part of the migration chain. |
| `db/templates/organization_private.sql` | Template for new private organization tables (forced RLS, root FKs, grants). |
| `testdata/` | Load seed, e2e rehearsal and forum source fixtures (synthetic only). |
| `tools/staging-*` | Narrow staging fixtures/repairs; never run against production. |

Module groups (names under `internal/modules/`):

- Public and community: `directory` (canonical stations), `official` (ANP
  revisions), `quality`, `charging`, `mapdata`, `mapdist`, `exploration`,
  `community`, `trust`, `review`, `forum`, `feedback`, `moderation`, `safety`,
  `metrics`, `alerts`.
- Accounts and personal data: `identity` (proof-of-possession profile),
  `account`, `contributor`, `evidence` (private photo uploads), `privacy`,
  `cloudsync`, `driver`, `ledger`, `payments`, `subscription`, `entitlement`.
- Operators: `admin`, `staff`.
- Abastevo Empresas (private organizations): `organization`, `membership`,
  `orgaccess`, `companysession`, `orgcapability`, `orgaudit`, `orgevent`,
  `orgsync`, `orgtime`, `orgvalue`, `orgmedia`, `orgnotify`, `orgexport`,
  `orgamend`, `fleet`, `fueling`, `expense`, `budget`, `trip`, `tracking`,
  `maintenance`, `document`, `stationprofile`, `enterpriseexport`; `kernel` is
  the shared value kernel.

## Run locally

```sh
docker compose -f infra/compose.dev.yml up -d db   # PostGIS on 127.0.0.1:5434 (repo root)
cd backend
go run ./cmd/migrate
go run ./cmd/api      # :8080; development defaults to the loopback compose DB
go run ./cmd/worker
```

`backend/.env.example` documents the local keys. For the Android map-probe
build talking to a local API (`./gradlew :app:assembleMapProbe
-PmapApiLocalProof=true`, `adb reverse tcp:18080 tcp:18080`), development
alone accepts a loopback `host:port` canonical host:

```sh
ANPFUEL_ENV=development ANPFUEL_HTTP_ADDR=127.0.0.1:18080 \
ANPFUEL_CANONICAL_HOST=127.0.0.1:18080 ANPFUEL_FORUM_ENABLED=true go run ./cmd/api
```

Images: `docker build -f infra/docker/Dockerfile --target <api|worker|migrate|station-load> .`
from the repository root (static binaries on distroless, user 65532; the worker
image also carries the Rust preparers).

## Test

| Scope | Command |
|-------|---------|
| Unit | `cd backend && go test ./...` |
| Fast gate, no DB | `bash scripts/check-backend-fast.sh` (gofmt, build, vet, unit, staticcheck, `sqlc vet`/`sqlc diff`, OpenAPI lint, secret scans) then `cd backend && govulncheck ./...` |
| Change-aware gate | `make quick-verify` (the required `Quick verification` check) |
| Contracts | `cd backend && go test -count=1 ./internal/platform/apicontract` |
| Integration | see below; real PostGIS, race detector |
| Full local matrix | `make test-local-backend` (`scripts/tests/test-local-backend.sh`) |
| Local TLS edge | `ANPFUEL_LOCAL_API_URL=http://<loopback-or-private-ip>:<port> make test-local-edge` |

Integration tests against a disposable PostGIS (from the repository root):

```sh
cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml \
  --bin prepare_registry --bin prepare_quality --bin prepare_sanctions \
  --bin prepare_fine_history --bin prepare_enforcement --target-dir /tmp/abastevo-rust
R=/tmp/abastevo-rust/debug
chmod 755 $R/prepare_registry $R/prepare_quality $R/prepare_sanctions $R/prepare_fine_history $R/prepare_enforcement
export ANPFUEL_TEST_PREPARER_PATH=$R/prepare_registry
export ANPFUEL_TEST_QUALITY_PREPARER_PATH=$R/prepare_quality
export ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=$R/prepare_sanctions
export ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=$R/prepare_fine_history
export ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH=$R/prepare_enforcement
docker compose -f infra/compose.dev.yml up -d db
cd backend
export ANPFUEL_TEST_DATABASE_URL='postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable'
ANPFUEL_DATABASE_URL="$ANPFUEL_TEST_DATABASE_URL" go run ./cmd/migrate
go test -race -count=1 -tags=integration ./internal/modules/<module>/...   # or ./... (long)
```

- `ANPFUEL_TEST_DATABASE_URL` must be a disposable PostGIS reached through a
  literal `127.0.0.1` DSN; many suites refuse any other host. Suites create and
  drop their own databases and roles, so the login needs CREATEDB/CREATEROLE.
  Never point it at staging or any persistent database.
- Integration fails, never skips, when PostGIS, a preparer or storage is
  missing. Real-storage suites (`orgmedia`, evidence) also need
  `ANPFUEL_TEST_STORAGE_{ENDPOINT,BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}`;
  `infra/compose.validation.yml` (unique project, `storage-init`) provides them.
- The preparer binaries must not be group/world-writable (the trusted bridge
  refuses them), hence the `chmod`.
- CI (`.github/workflows/backend.yml`) splits integration into parallel legs:
  `organization/adapters` in 4 shards, `platform/apicontract` in 2, everything
  else in one. Reproduce a leg with
  `python3 scripts/backend-integration-shard.py --package ./internal/modules/organization/adapters --shard 0 --shards 4`
  or `--rest`. The matrix and `SHARDED` must agree
  (`python3 scripts/tests/test-backend-integration-shard.py`).
- Use `-count=1` after editing OpenAPI files or vectors: the Go test cache does
  not track them. `go test -run '^$'` only compiles and is never test evidence.
- `ANPFUEL_TEST_RUNTIME_GRANTS=script` reruns the orgtest suites and the API
  process tests as a role provisioned by `infra/db/runtime-role.sql`.

Tool pins (match CI): sqlc `v1.31.1`, staticcheck `v0.8.1` and govulncheck
`v1.8.0` installed and run under `GOTOOLCHAIN=go1.27.1` (staticcheck cannot
read newer export data yet), vacuum `v0.30.6`, Rust `1.99.0`. Format with the
go.mod toolchain's gofmt, not an older system one:
`"$(cd backend && go env GOROOT)/bin/gofmt" -l backend/ contracts/`
(prefix `PATH` with that directory before `make quick-verify`).

## Configuration

Environment variables only (names below; values live in host secrets, never in
Git). `internal/platform/config` validates everything at startup and refuses
unsafe combinations; staging and production require an explicit DSN.

- Core: `ANPFUEL_ENV` (`development` default, `staging`, `production`),
  `ANPFUEL_LOG_LEVEL`, `ANPFUEL_HTTP_ADDR`, `ANPFUEL_DATABASE_URL`,
  `ANPFUEL_MAX_BODY_BYTES`, `ANPFUEL_CURSOR_SECRET` (shared across replicas),
  `ANPFUEL_CANONICAL_HOST`, `ANPFUEL_TRUSTED_PROXIES`,
  `ANPFUEL_PUBLIC_WEB_ORIGINS`, `ANPFUEL_METRICS_ADDR` (loopback-only private
  listener; empty disables), `ANPFUEL_MAIL_MODE`,
  `ANPFUEL_GLOBAL_QUOTA_{CHALLENGE,REGISTER,WRITE}`.
- Shutdown: `ANPFUEL_SHUTDOWN_DRAIN_DELAY` (default 10s, 0–30s: readiness turns
  503 while traffic still flows) and `ANPFUEL_SHUTDOWN_TIMEOUT` (default 10s).
  Keep delay + timeout + 5s below the pod's `terminationGracePeriodSeconds`.
- Private storage: `ANPFUEL_R2_{ENDPOINT,BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}`
  (all or nothing; unset disables upload issuance; HTTPS when enterprise is on),
  `ANPFUEL_DEV_STORAGE_SERVICE`, `ANPFUEL_DEV_PHOTO_PREVIEW_UNTIL`,
  `ANPFUEL_EVIDENCE_MAX_PENDING_PHOTOS` (default 0 = intake closed; same value
  on every replica), `ANPFUEL_TRAINING_TERMS_DIGEST`.
- Admin API (`/v1/admin/*` mounts only with key hash and origin):
  `ANPFUEL_ADMIN_KEY_SHA256`, `ANPFUEL_ADMIN_KEY_SHA256_NEXT` (rotation),
  `ANPFUEL_ADMIN_ORIGIN`, `ANPFUEL_ADMIN_ALLOWED_CIDRS`.
- Feature switches: `ANPFUEL_FORUM_ENABLED`, `ANPFUEL_DRIVER_ENABLED`,
  `ANPFUEL_ANP_DISCOVERY_ENABLED`,
  `ANPFUEL_PRICE_RANKING_{ENABLED,CACHE_ENTRIES,CACHE_TTL_SECONDS}`,
  `ANPFUEL_CHARGING_{READS,IMPORTS}_ENABLED`,
  `ANPFUEL_QUALITY_{READS,COMPANY_READS,REPORTS,IMPORTS,DISCOVERY,REVALIDATION,SANCTIONS_DISCOVERY,ENFORCEMENT_IMPORTS,ENFORCEMENT_DISCOVERY}_ENABLED`.
  Source imports and schedules are independent and default off; a schedule
  only enqueues work.
- Enterprise: `ANPFUEL_ENTERPRISE_ENABLED` (default false),
  `ANPFUEL_ENTERPRISE_DEVICE_LIMIT` (1–5, default 3),
  `ANPFUEL_ENTERPRISE_METER_PLAUSIBILITY` (rollback switch),
  `ANPFUEL_ENTERPRISE_EXPORT_{ENABLED,ROOT,WINDOW_WEEKS}`,
  `ANPFUEL_ENTERPRISE_PORTAL_{ROOT,SITE_URL,LANDING_URL,SEARCH_JS,INDEXABLE,OFFICIAL_ONLY,REFRESH_ENABLED}`.
- Accounts: `ANPFUEL_APPLE_NATIVE_AUDIENCE` (iOS bundle ID; unset keeps native
  Sign in with Apple answering 503, never `anpfuel-backend`);
  `ANPFUEL_APPLE_{TEAM_ID,KEY_ID,PRIVATE_KEY_FILE,GRANT_KEY_FILE}` (all or
  none, absolute file paths, read by API and worker at start) enable Apple
  token revocation on account deletion.
- Payments: `ANPFUEL_PAYMENTS_{CATALOG_VERSION,RETURN_ORIGIN}`, `ANPFUEL_STRIPE_*`,
  `ANPFUEL_ASAAS_*` (modes, keys, webhook secrets, per-method sale switches).
- Maps and misc: `ANPFUEL_MAP_DIST_DIR`, `ANPFUEL_MAP_DIST_PUBLIC_KEYS`,
  `ANPFUEL_GITHUB_REPO`, `ANPFUEL_GITHUB_TOKEN`; `ops` reads
  `ANPFUEL_OPERATOR_ID`, `ANPFUEL_MONITORING_WEBHOOK`, `ANPFUEL_BACKUP_MANIFEST`.

## Rules an agent must not break

- `domain/` packages are pure: no I/O, SQL, HTTP or provider calls. Cross-module
  access goes through explicit ports; no module imports another module's
  `db/queries` package, and each `sqlc.yaml` stanza lists only its owner's
  migrations.
- SQL lives in `db/queries` and is parametrized through sqlc. After editing,
  run `sqlc vet && sqlc generate` from `backend/` and commit the output; the
  fast gate fails on drift. Never build SQL from strings (generated identifiers
  must be quoted with `pgx.Identifier`).
- Migrations are append-only: never edit, rename or delete an applied file; add
  the next number. No destructive down migrations; rollback is a forward
  redeploy and the previous image must keep working (expand, then contract).
  Raising `schema_min_supported` is a contract change the deploy agent never
  applies automatically.
- After any migration or SQL change, regenerate and review the runtime grant
  inventory: `ANPFUEL_RUNTIME_GRANTS_WRITE=1 go test -tags=integration -count=1
  -run TestRuntimeScriptMatchesHead ./internal/platform/runtimegrants` (without
  the variable the test fails on a stale `infra/db/runtime-role.sql`). Never
  hand-edit that script or grant TRUNCATE, DDL, ownership or SECURITY DEFINER.
- API and worker connect as an ordinary login (NOSUPERUSER, NOBYPASSRLS, owns
  no table). Organization tables use forced RLS; never weaken it to make a
  query work.
- Public reads may be anonymous. Signed writes and private owner reads require
  proof of possession bound to `ANPFUEL_CANONICAL_HOST`; proxy hosts are
  untrusted. Staging/production canonical hosts are bare public hostnames.
- Official sources (ANP, registry, quality, sanctions) and community data stay
  distinct: official endpoints refuse community data, never substitute it.
  Payment never affects trust or free participation.
- The organization boundary applies only to Abastevo Empresas data. Its routes
  mount only with `ANPFUEL_ENTERPRISE_ENABLED`; per-organization capabilities
  default to deny and change only through `ops enterprise capability set`.
  Every organization write route declares a `WriteAdmission` (registration
  fails otherwise), re-checks current membership, permission and fresh proof,
  and carries an operation ID plus expected revision; replays return the
  original acknowledgement and receipts are never deleted.
- Lock order: accounts in UUID order, then organization roots in UUID order.
  `WithOrgTx` runs READ COMMITTED. No external call (storage, provider, HTTP)
  inside a database transaction or while holding these locks.
- Job handlers tolerate redelivery and concurrent workers: the effect commits
  with its receipt in one transaction; the dispatcher owns acknowledgement.
- Times are stored as UTC instants with server receipt time; reporting
  day/zone snapshots are captured at write and never recomputed. Resolve zones
  with `kernel.LoadTimeZone`.
- Logs and errors carry no secrets, tokens, PII, precise contributor GPS or
  raw provider/SQL errors. DTOs redact `fmt`/slog/JSON output; never pass raw
  URLs to request logging. Fixtures are synthetic.
- New endpoints go into `contracts/openapi/v1.yaml` with vectors before the
  handler; a published contract is never reinterpreted.
- New dependencies need a recorded need, license check and security review.

## Operations

- `ops` subcommands: `directory`, `quality`, `subscription`, `feedback`,
  `moderation`, `privacy erase|replay`, `monitoring eval`, `evidence-url`,
  `admin keygen` (offline; prints a key and its SHA-256 once),
  `meter rebuild-projection --org <uuid>`, `enterprise capability show|set`.
  Run `go run ./cmd/ops help` for flags; in staging run it inside the `api`
  container (`/ops`) with that container's runtime DSN.
- Offline station tools use `ANPFUEL_DATABASE_URL`, run no migrations and
  replay an exact source snapshot idempotently.
- Staging runs on VPS 72.60.147.2 at https://abastevo.com.br, in k3s namespace
  `abastevo-temp`, deployed only by the signed pull agent
  ([agent](../infra/deploy/agent/README.md)). Its database is not a test
  database.

## Further reading

[Architecture](../docs/architecture/backend.md) ·
[API](../docs/backend/api.md) ·
[Domain model](../docs/backend/domain-model.md) ·
[Data model](../docs/backend/data-model.md) ·
[Testing](../docs/backend/testing.md) ·
[Toolchain](../docs/backend/toolchain.md) ·
[Operations](../docs/operations/) ·
[Security and privacy](../docs/security/security-privacy.md) ·
[Enterprise](../docs/enterprise/README.md) ·
[Agent guide](../AGENTS.md)
