# F1 EMP-PLAT-001 organization value contract

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING. F1 remains IN_PROGRESS.
Task EMP-PLAT-001 follows the accepted M0 checkpoint `39a2d725` on
`codex/enterprise-f0-f1`, based on synchronized main/dev `d8bdc642`.
The checkout is isolated because maintained local dev contains unrelated
commits. Protected delivery through dev remains pending.

## Entry and behavior

The [F0 checkpoint](f0-local-checkpoint.md) records the complete plan
verification and actual integration of the ADR-019 source batch through
PR177. Its successful checks and ancestry satisfy the local F1 entry.

B-BR-EMP-08 and BUC-EMP-04 were recorded before implementation.
The existing shared kernel now provides a validated non-nil UUID `OrgID`
and an immutable `OrgScope`. Parsing accepts either hex case and returns
the canonical lowercase dashed value. Empty, padded, malformed and nil
identifiers fail with a bounded error. Scope construction and validation
reject zero values. Selecting another organization leaves earlier scope
values intact.

The [repository convention and module checklist](../09-architecture.md#21-private-organization-repository-contract)
require explicit scope on private ports, adapter validation, scoped
references and current authorization at the application boundary. They
also identify the owning database, job, storage and entry-point tests.

## Tested source and environment

Linux amd64, Go 1.27.3 from the backend module. The final source blobs are
`36e63ad173048d5b4b18f8e055a092933be0fc0e` for `org_scope.go` and
`c39b40908a9e6f57501e0f05b26d1c9a8d0e0e16` for `org_scope_test.go`.
The source and this evidence are committed together above the F0 checkpoint.

The first Go command could not write the default cache. Subsequent commands
used `GOCACHE=/tmp/enterprise-go-build`. Staticcheck v0.8.1 initially hit
the documented newer-compiler export-format incompatibility, then passed
under `GOTOOLCHAIN=go1.27.1` as prescribed in
[backend/README](../../../backend/README.md).

## Executed checks

- RED reproduced the absent types and functions with
  `go test ./internal/modules/kernel -run 'Test(ParseOrgID|OrgScope)' -count=1`.
- A further malformed UUID vector with two extra separators failed before
  the exact 32-hex-character check was added.
- `go test -race -coverprofile=/tmp/enterprise-kernel-coverage.out ./internal/modules/kernel`
  passed, including the existing standard-library-only dependency check.
  The 17 parsing vectors and scope tests cover zero values, canonical
  round trips and independent organizations.
- `go tool cover -func=/tmp/enterprise-kernel-coverage.out` reports 94.4%
  package coverage and 100% for all five new functions/methods.
- `go vet ./internal/modules/kernel` passed.
- `XDG_CACHE_HOME=/tmp/enterprise-staticcheck GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/kernel`
  passed with the pinned v0.8.1 binary.
- `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- Explicit `gofmt` listing, enterprise checker and its regression tests,
  `git diff --check`, new-file whitespace and added-content secret review
  passed.

## Next acceptance work

EMP-PLAT-002 owns the schema conventions and real-PostGIS two-organization
isolation harness, including ordinary roles, absent scope, foreign references,
pool reuse and concurrency. Application authorization follows in the AUTH
tasks. These value types carry selection and enforce valid construction.
Runtime privacy acceptance requires those subsequent layers and tests.

F1/M1 remains open. Real-receipt legal review, P38 device acceptance,
Stage D, provider/billing and production gates retain their recorded owners.
