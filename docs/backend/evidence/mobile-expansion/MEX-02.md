# MEX-02 — Typed exploration and official-record presentation contract

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-21
Parent: d61ccc4c; dependencies MEX-00 and MEX-01 accepted locally.

## Before behavior

Scope: B-BR-MEX-08–18, BUC-MEX-06–10; typed price/official query examples,
ambiguity and available-filter behavior, source/company/site meaning, clocks,
coverage, snapshot identity, Android navigation/copy and compatibility cases.
Risk: public factual attribution, misleading price rankings and private leakage.
Reuse FQ leases/company scope, PRM privacy and OPT cache/freshness contracts.

Acceptance will validate a separate planned schema (no live path), positive and
negative examples: cheapest versus average ambiguity, L/M3 mismatch, unknown
fuel, repeated snapshots, distinct results sharing a document, unmatched company,
unavailable source, no-price station, offline association and source clocks.
No SQL/parser/search engine/new score, public private-report route, live API,
mobile screen or EV contract from EE-02 is implemented here.

Planned checks: schema/fixture RED→GREEN; complete Go apicontract race plus
affected build/vet/static/gofmt; source vocabulary cross-check, document links,
secret review and diff --check. Exact data queries/locking/page/HTTP acceptance
remain in the owning downstream units. Recovery: forward unpublished contract
correction without changing a live API or public attribution. No publication.

## Delivered and verified

[Exploration contract](../../../product/MOBILE_EXPLORATION_CONTRACT.md) freezes
visible typed interpretations, exact source/unit/condition/metric, source versus
current-company geography, source clocks/precision, coverage/unknown states,
identity/counting, filter capabilities, cursor/cache guards and five Android
journeys with reviewed Portuguese copy. Actual resources/screens remain later.

[Planned schema](../../../../contracts/openapi/mobile-exploration-planned.json)
has 20 components, zero paths and explicit runtime-pending status; live v1 is
unchanged. [Shape examples](../../../../contracts/testdata/mobile-expansion/exploration-v1.json)
contain 54 cases (26 valid, 28 rejected); [counting examples](../../../../contracts/testdata/mobile-expansion/counting-v1.json)
contain three cases. They distinguish highest observation/area mean, conditional
prices, CNG M3/L, unknown fuel/schema, company/site claims, unavailable/null versus
empty/zero, no-price stations, private-field rejection and actual source clocks.
CONTENT_OBSERVATION cannot manufacture an issued-case ID; SOURCE_ISSUED_CASE
requires one. A repeated snapshot never doubles the selected-record count, and
distinct results sharing a document stay distinct. Issued-fine totals stay null
without source case evidence. No real-source/census or national-coverage claim.

RED: `go test -count=1 ./internal/platform/apicontract -run '^TestMobileExplorationPlannedContract$'`
failed because the selected contract file did not exist. GREEN: focused schema/
counting examples PASS, then `go test -race -count=1 ./internal/platform/apicontract`
PASS (8.324s), including MEX-01 and all existing live API goldens. `go vet`,
`GOTOOLCHAIN=go1.27.1 staticcheck` and gofmt for the affected tests PASS. This
tests-only package is compiled by the executed test run, not go build.
One intermediate rerun hit a read-only Go cache; the final identical scoped
validation ran with normal cache escalation and has no outstanding failure.

Document/JSON checks: 37 relative targets before final navigation edits, unique
fixture IDs/counts, explicit unknown/current-company semantics and zero live
paths; final navigation scan resolves 439 relative targets. Repository secret
scan and redacted gitleaks scan of the full staged diff report no leaks. Diff review and
git diff --check PASS. Production source, SQL/Room, existing live API and source
data stay unchanged. No PostGIS/SQLite/native/latency/HTTP-pipeline acceptance is
claimed: those require the downstream implementation, including calendar/UF/
source applicability cross-field guards that JSON shape checks do not establish.

Requested two-task batch ends here. Next in the declared sequence: MEX-03
performance baseline; EE-00 is independently ready but unexecuted. EE contracts,
live provider permission, real source acceptance, D and release gates remain
pending. No remote publication/merge/deployment or device action occurred.
