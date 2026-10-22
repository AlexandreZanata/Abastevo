# MEX-20d — Atomic publication and frozen source reads

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

State: LOCAL_DONE / INTEGRATION_PENDING. Parent:2206b31e. Rules B-BR-MEX-10/11/12/16,
BUC-MEX-07. Reuse edition publication identity, guard, transaction, correction
and immutable history. Native source geography/document/procedure/result are
explicit fields; unknown occurrence/as-of/extraction stay null/unavailable.
No identity, station attribution or inferred legal finality in public reads.
Legacy filters do not invent native status/scope. Native context is an optional
publication-key extension so old keys remain byte-compatible.

## Executed evidence

The pre-fix read returned zero native facts from a published nonempty revision;
its regression now passes. Publication uses the stable dataset edition
`enforcement-source-v1` (not a coverage clock), which also permits the actual
legacy YYYY-MM current pointer to advance. Context joins the optional publication
identity; 128-record SQL chunks remain inside the shared guarded transaction.
Four writers converge; replay/live guard, correction, frozen history, 129-record
traversal, second-chunk ID failure/full rollback and empty snapshots PASS.

Actual HTTP page/detail match OpenAPI, preserve long/distinct source results,
refuse legacy status/scope and station filters, bind old cursors to revisions,
and emit null occurrence/as-of/status/scope with an UNAVAILABLE extraction clock.
No company identifier is public. Local publication never makes this source fresh.

Affected race/PostGIS suites: adapters45.6s, reads31.4s, HTTP26.6s, application
and API contracts PASS. The first matrix exposed a missing required Rust CLI
environment and the empty-date regression; both were corrected and affected
tests rerun successfully. Vet/staticcheck, deterministic sqlc vet/diff, diff and
secret review PASS; OpenAPI0 errors (414 existing warnings/103 informs).
No worker registration, network activation, dependency or release.
