# Backend workspace

Status: planning only. No Go module, dependencies, production migrations or API implementation have been created.

Start with [P01-T01](../ROADMAP.md#p01-t01), read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md). Implementation will add `cmd/api`, `cmd/worker`, `cmd/migrate`, owned business modules, explicit SQL and reproducible development commands incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.
