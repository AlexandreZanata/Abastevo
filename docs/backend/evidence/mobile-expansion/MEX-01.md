# MEX-01 — Vehicle registration and retained-data contract

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-21
Parent: 0e7af9d5, codex/mobile-expansion-plan (clean at opening).

## Before behavior

Authorized scope: MEX-01 and then MEX-02 only. MEX-00 is accepted locally;
maintained dev remains a2f8925c, already an ancestor. Foreign work is preserved.
B-BR-MEX-01/02/06 and BUC-MEX-01/04 govern one free registration, additional
verified Premium registrations within ceiling3 and preserved held data.
PRM00/02 remain the owners. Risk: money/access/privacy and legacy compatibility.

This task freezes a staged machine-policy amendment and worked examples. It
does not change runtime admission, subscription grants, SQL/Room or live APIs.
Targeted acceptance: free first/second; Premium third/fourth; retained legacy2/3;
lapse/correction/replay; hosted versus offline rights; expiry/rollback; serialized
concurrent admission and import/restore quota examples. Future real persistence
concurrency/rollback acceptance remains mandatory in MEX-07/08.

Planned checks: contract RED then GREEN with exact nonempty Go test selectors;
complete apicontract and affected subscription domain checks with race; affected
Go build/vet/gofmt; relative links, scoped secrets and git diff --check. No DB
test is claimed for this contract-only change; no storage/runtime code changes.

Recovery: forward contract correction, preserve all v1 fields and legacy data;
MEX-07/08 must consume the amendment before any activation. No publication.

## Delivered and verified

[Registration v2](../../../product/VEHICLE_REGISTRATION_POLICY.md) freezes new
`vehicles.multiple`, distinct non-deleted-ID counting (including archived held
registrations), private/local versus hosted authority, bounded offline lease,
atomic import/restore recheck and legacy retention. Existing policy-v1 fields
are byte-value compatible; the nested v2 amendment is explicitly runtime-pending.
Canonical PRM/product/community/source contracts now align with the selected
one-free policy. Professional/cloud/financial rights and ceiling3 are unchanged.

RED: `go test -count=1 ./internal/platform/apicontract -run '^TestMobileVehicleRegistrationContract$'`
failed on the absent amendment. GREEN: 23 reviewed examples plus the complete
apicontract and subscription domain suites pass with
`go test -race -count=1 ./internal/platform/apicontract ./internal/modules/subscription/domain`
(5.047s / 1.009s). The example checker is an arithmetic contract oracle, not a
replacement for actual multi-writer persistence/lease tests in MEX-07/08.

`go vet` for both affected packages, `go build ./internal/modules/subscription/domain`,
`GOTOOLCHAIN=go1.27.1 staticcheck` for both packages and gofmt PASS.
Go's tests-only apicontract package cannot be passed to `go build`; that attempted
command failed with "no non-test Go files". Its compilation is covered by the
actual test run; the corrected build selector above passed. Initial cache writes
hit sandbox read-only restrictions; the same scoped checks passed with normal
cache escalation. No known test/static failure remains.

Document checks: 134 relative file targets; original v1 values invariant and
sales/provider/admission OFF; all 23 fixture IDs; no conflicting three-free wording
in affected canonical contracts; secret-surface review/scan and diff --check PASS.
Only contracts, documentation and contract tests changed. No production/domain
implementation, migration, activation, remote action or device test is claimed.
Next authorized microtask: MEX-02; EE-00 and MEX-03 remain unexecuted.
