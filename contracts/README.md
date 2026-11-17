# Contracts

Language-neutral contracts shared by the Go backend, the Kotlin app and the
Rust preparers: the HTTP API schema, versioned policy/format files and golden
vectors that every implementation must reproduce. No code lives here.

## Layout

| Path | Contents |
|------|----------|
| `openapi/v1.yaml` | The `/v1` HTTP API (OpenAPI 3.1), linted with `openapi/vacuum-rules.yaml`. |
| `openapi/*-planned.json` | Proposed APIs (`x-implementation-status: planned`), not live endpoints. |
| `testdata/<area>/` | Versioned golden vectors (API, identity, ANP, quality, registry, map, enterprise, payments, …). |
| `admin/`, `ev/`, `map/`, `premium/`, `station-prep/` | Versioned catalogs, profiles, JSON schemas and source-format specs. |

Consumers: `backend/internal/platform/apicontract` and module tests (Go),
`domain/` tests (Kotlin) and `tools/station-prep` (Rust).

## Checks

From the repository root:

```sh
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && go test -count=1 ./internal/platform/apicontract/...)
./gradlew :domain:test          # Kotlin vector consumers
make check-rust                 # Rust consumers
```

`vacuum` is pinned at `v0.30.6` (`go install github.com/daveshanley/vacuum@v0.30.6`).
Keep `-count=1`: the Go test cache does not track these files. Both checks run
in `scripts/check-backend-fast.sh` and `make quick-verify`.

## Rules

- Contract first: add or change the schema and its vectors before the handler.
- A published contract is never overwritten, only superseded by a new version
  (`-v1`, `-v2`, …). No silent enum rename or unit/amount reinterpretation.
- Every fixture carries a format version, stable ID and provenance, and uses
  synthetic identities, images and locations only.
- `*-planned.json` files are proposals; live endpoints are specified in
  `v1.yaml`.

Behavior and vocabulary: [API](../docs/backend/api.md) ·
[Domain model](../docs/backend/domain-model.md) ·
[Testing](../docs/backend/testing.md).
