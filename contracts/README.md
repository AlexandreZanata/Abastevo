# Shared contract workspace

Status: planned contracts, no executable OpenAPI/fixture set yet. [API_PLAN](../docs/backend/API_PLAN.md) defines behavior; P01-T11 introduces versioned OpenAPI before HTTP business endpoints. [ANP_INGESTION](../docs/backend/ANP_INGESTION.md) specifies shared golden fixtures for P02.

Planned directories: `openapi/`, `testdata/anp/`, `testdata/api/`, `testdata/identity/`, `testdata/consensus/`. Each fixture has format version, stable ID, source/provenance, expected values and explicit legacy Android differences. Use synthetic identities/images only. Backend consumes fixtures first; Kotlin harnesses begin after G09 in P10.
