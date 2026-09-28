# Backend workspace

Status: P01-T02 roots only. Go module (`go 1.27`, toolchain `go1.27.1`) with
minimal `cmd/api`, `cmd/worker` and `cmd/migrate` composition roots exists and
compiles (`go test ./... && go build ./cmd/...` from `backend/`). No
dependencies, production migrations or API implementation have been created.

Tooling pins: Go toolchain `go1.27.1` (`go.mod`); `sqlc v1.31.1`
(`sqlc vet && sqlc generate` from `backend/`; generated packages committed).
`chi v5.3.2`, `pgx v5.11.0` (both MIT, permissive transitives only).
`staticcheck v0.8.1` (MIT), `govulncheck v1.8.0`, `vacuum v0.30.6` (MIT).
Static binaries must be built with Go ≥1.27 or typechecking fails; install
with `GOTOOLCHAIN=go1.27.1 go install <tool>@<version>`.

## Gates

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
cd backend && ANPFUEL_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable go run ./cmd/migrate
cd backend && go test -race -count=1 -tags=integration ./...
```

Integration fails when PostGIS is unreachable; it never skips. CI
(`.github/workflows/backend.yml`) runs both gates path-aware on backend,
contract, infra and gate-script changes; the Android workflow keeps running
`./gradlew test` on Android paths and skips pure docs/contract/backend edits.

Start with [P01-T01](../ROADMAP.md#p01-t01), read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md). Implementation will add `cmd/api`, `cmd/worker`, `cmd/migrate`, owned business modules, explicit SQL and reproducible development commands incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.
