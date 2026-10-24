# Shared contract workspace

Status: base OpenAPI v1 (draft, unpublished) plus golden API vectors since
P01-T11. [API_PLAN](../docs/backend/API_PLAN.md) defines behavior; domain
endpoints land contract-first in later tasks. [ANP_INGESTION](../docs/backend/ANP_INGESTION.md) specifies shared golden fixtures for P02.

Directories: `openapi/`, `testdata/anp/`, `testdata/api/`, `testdata/identity/`, `testdata/consensus/`, `testdata/map-discovery/`. Each fixture has format version, stable ID, source/provenance, expected values and explicit legacy Android differences. Use synthetic identities/images only. Backend consumes fixtures first; Kotlin harnesses begin after G09 in P10.

`testdata/map-discovery/cases-v1.json` freezes MAP-01 source-mode, unknown-price,
combined-site, missing-geometry, source-race and rights-blocked behavior. It is a
contract fixture, not a published API schema.

## Pinned checks (run from the repository root unless noted)

```sh
go install github.com/daveshanley/vacuum@v0.30.6   # one-time; MIT
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
cd backend && go test ./internal/platform/apicontract/...  # golden vectors vs schemas
```

Rules: unpublished draft only; never overwrite a published contract; new
endpoints are added to `openapi/v1.yaml` with vectors before handlers exist.
