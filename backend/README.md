# Backend workspace

Status: P01-T02 roots only. Go module (`go 1.27`, toolchain `go1.27.1`) with
minimal `cmd/api`, `cmd/worker` and `cmd/migrate` composition roots exists and
compiles (`go test ./... && go build ./cmd/...` from `backend/`). No
dependencies, production migrations or API implementation have been created.

Start with [P01-T01](../ROADMAP.md#p01-t01), read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md). Implementation will add `cmd/api`, `cmd/worker`, `cmd/migrate`, owned business modules, explicit SQL and reproducible development commands incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.
