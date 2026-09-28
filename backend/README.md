# Backend workspace

Status: P01-T01…T12 foundations exist: API/worker/migrator roots, typed config,
redacted telemetry, HTTP/health lifecycle, pgx pool, migration runner, sqlc,
container definitions and base OpenAPI vectors. Business modules and
production infrastructure remain ahead. See [current state](../docs/planning/PROGRESS.md)
and [prior task evidence](../docs/planning/history/P01_FOUNDATION_EVIDENCE.md);
this documentation revision does not rerun those suites or verify remote CI.

Tooling pins: Go toolchain `go1.27.1` (`go.mod`); `sqlc v1.31.1`
(`sqlc vet && sqlc generate` from `backend/`; generated packages committed).
`chi v5.3.2`, `pgx v5.11.0` (both MIT, permissive transitives only).
`staticcheck v0.8.1` (MIT), `govulncheck v1.8.0`, `vacuum v0.30.6` (MIT).
Static binaries must be built with Go ≥1.27 or typechecking fails; install
with `GOTOOLCHAIN=go1.27.1 go install <tool>@<version>`.

## Existing gates and planned cadence

The commands below exist today. G01-FLOW will introduce shared quick/full
entry points and safely migrate CI/protection settings; `make quick-verify`
and `scripts/git-flow.sh` are still planned. Use [CI_PLAN](../docs/planning/CI_PLAN.md)
for focused task checks, phase integration and complete release certification.
Existing workflow requirements apply until activation is proven.

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
