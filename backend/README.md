# Backend workspace

Status: P01-T01…T12 foundations exist: API/worker/migrator roots, typed config,
redacted telemetry, HTTP/health lifecycle, pgx pool, migration runner, sqlc,
container definitions and base OpenAPI vectors. Business modules and
production infrastructure remain ahead. See [current state](../docs/planning/PROGRESS.md)
and [prior task evidence](../docs/planning/history/P01_FOUNDATION_EVIDENCE.md);
this documentation revision does not rerun those suites or verify remote CI.

Tooling pins: Go toolchain `go1.27.2` (`go.mod`); `sqlc v1.31.1`
(`sqlc vet && sqlc generate` from `backend/`; generated packages committed).
`chi v5.3.2`, `pgx v5.11.0` (both MIT, permissive transitives only).
`staticcheck v0.8.1` (MIT), `govulncheck v1.8.0`, `vacuum v0.30.6` (MIT).
Static binaries must be built with Go ≥1.27 or typechecking fails; install
with `GOTOOLCHAIN=go1.27.2 go install <tool>@<version>`. The pinned
staticcheck predates the go1.27.2 export format, so the fast gate runs
`staticcheck` itself under `GOTOOLCHAIN=go1.27.1` (see
`scripts/check-backend-fast.sh`).

## Verification entry points (P01-T13)

Shared local/remote composition lives in `Makefile` and `scripts/`:

```sh
make quick-verify    # bounded gate: manifest + selection + fast checks + govulncheck
make verify-release  # quick + full-matrix report (foundation subset, NOT CERTIFIED)
make test-gate       # focused harness for selection/failure behavior
```

Manifest `scripts/check-manifest.txt` lists required quick packages and
checks; missing packages or zero matched tests fail. Docs-only changes run
docs checks plus contract reference integrity; code changes run the full
quick set. Unclassified paths, missing tools and secrets in working tree or
committed PR diff fail. `go test -run '^$'` is compile-only and never counts
as behavioral evidence. Measured quick budget is 300s warm cache.

Required CI is the always-on **Quick verification** job (`.github/workflows/quick.yml`):
it runs `make quick-verify` on every PR including drafts plus main pushes, with
no path filter, so docs-only changes still report a meaningful required result.
`backend.yml` fast/integration and `ci.yml` test keep running path-triggered as
non-required signal; their task-level and phase-exit evidence stays mandatory
per [CI_PLAN](../docs/planning/CI_PLAN.md).

Existing gates below remain binding until G01-FLOW T17 migrates CI/protection.

Fast gate, no database (from the repository root):

```sh
bash scripts/check-backend-fast.sh   # gofmt, build, vet, unit tests,
                                     # staticcheck, sqlc vet/diff, vacuum lint,
                                     # tracked + changed-file secret scans
cd backend && govulncheck ./...
```

Integration gate, real disposable PostGIS (from the repository root):

```sh
docker compose -f infra/compose.dev.yml up -d db
(
  cd backend
  export ANPFUEL_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
  export ANPFUEL_TEST_DATABASE_URL="$ANPFUEL_DATABASE_URL"
  go run ./cmd/migrate && go test -race -count=1 -tags=integration ./...
)
```

Integration fails when PostGIS is unreachable; it never skips. CI
(`.github/workflows/backend.yml`) runs both gates path-aware on backend,
contract, infra and gate-script changes; the Android workflow keeps running
`./gradlew test` on Android paths and skips pure docs/contract/backend edits.

Continue with [P01-T13](../ROADMAP.md#p01-t13); read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md) as the selected task requires. P02 starts after G01 and G01-FLOW; subsequent phases add owned business modules and explicit SQL incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.

## Complete local backend validation

From the repository root, `bash scripts/tests/test-local-backend.sh` creates a
unique disposable Compose project with tmpfs PostGIS and a private S3 emulator.
It runs all unit/integration suites under the race detector, actual API + worker
HTTP/media flows, static/vulnerability checks, infrastructure/deploy harnesses,
encrypted backup, restored deletion-ledger replay, bounded load and DB outage.
It clears inherited runtime DSN/storage settings and never selects the shared
`infra/compose.dev.yml` database. Only its own project is removed on exit.
Reports remain in a printed `/tmp` directory; set `ANPFUEL_LOCAL_REPORT_DIR` to
choose a local output path. The container credentials are synthetic test values.
These tmpfs services lose all data on removal; they are not production storage.

For an already running synthetic API on a local/private literal IP, run:

```sh
ANPFUEL_LOCAL_API_URL=http://172.19.2.11:18093 bash scripts/tests/test-local-edge.sh
```

This starts the pinned production Caddy image with an ephemeral internal CA,
validates TLS 1.2/1.3, health and route isolation, and checks synthetic location/
signature/cookie markers are absent from logs. It does not modify system trust
or obtain public certificates. The test edge and its keys are removed on exit.
Local test success is distinct from G09 certification and public deployment.
