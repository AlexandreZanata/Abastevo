# Premium first-ten execution evidence

Date: 2026-10-13. Branch: codex/premium-backend. Base: fetched main 5dc9b2b. Scope: PRM00-T01–T04 and PRM01-T01–T06; one commit per microtask; new PR to main. Primary dev/mobile edits excluded. No merge, deployment, paid activation or Android change is authorized.

## PRM00-T01 — Boundary and source reconciliation

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (documentation/source inspection)

The existing account service validates active account/session proofs; privacy/export/deletion, signed transport, PostgreSQL jobs, OpenAPI and private storage are reuse targets. Current main schema ends at 000060; separately progressing dev already uses through 000074. Reserve Premium migrations from 000075, avoiding known IDs; never rewrite applied migrations. Required main protection was read live: Quick verification. No CI result is inferred.

Free inventory: three local vehicles/tank estimates, ANP history/analysis, offline reading, city/station lookup/navigation, weekly local drop alerts, account recovery and social participation. New paid capabilities: cloud.sync/cloud.backup/cloud.restore/alerts.advanced/professional.write/professional.report. Consensus never consumes payment state. Individual records never become public observations automatically. No fleet tenants/extra paid vehicle slots.

User clarification: no Play subscription/payment module exists. Backend source/local fault testing is authorized; real provider/catalog/payment wiring/sandbox proof is deferred, sales OFF. Prices/trials/public activation require later configuration/release acceptance. Original planning commit e9b3c25 is incorporated within this task rather than importing unrelated dev/mobile source ancestry.

Checks: source/free-inventory review, local document links, repository baseline/secret guard and staged whitespace/changed-document secret review. No runtime suite claimed for this documentation task. Next: PRM00-T02.

## PRM00-T02 — Lifecycle, retention and bounded source policy

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (source-policy scenario review)

Frozen PREMIUM_SOURCE_POLICY and policy-v1.json with exact source ceilings, state/access mapping, 900s fresh proof, bounded offline lease, expiry/export/deletion policy, snapshot/queue/device/request limits and token key ownership. Reviewed cancel-paid-through, pending/unknown/outage, revoke during work, stale device, owner switch and old-backup resurrection scenarios. Admission remains zero; limits are conservative local source bounds, explicitly not measured VPS/paid capacity. Prices/catalog/credentials/provider proofs remain deferred by user.

Checks: JSON parse, positive bounds, zero admission/sales flags, no raised free vehicle allowance, staged diff/secret review. No retention/job/provider behavior is claimed implemented by this policy task. Next PRM00-T03.

## PRM00-T03 — Financial method and golden vectors

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS

RED: TestPremiumFinancialGoldenContract failed with missing golden file. GREEN: 15 synthetic reviewed cases; exact rational consumption/cost, partial-fill baseline exclusion, actual discount totals, weighted vehicles and explicit unsupported/gap/reset/month-window/overflow reasons. Money is centavos, quantity milli-units, distance metres; display half-up only. Financial engine remains a later owning task.

Commands: go test -race -count=1 ./internal/platform/apicontract -run '^TestPremiumFinancialGoldenContract$'; go vet ./internal/platform/apicontract; GOTOOLCHAIN=go1.27.1 staticcheck ./internal/platform/apicontract. Passed. Scoped whitespace/secret review. Next PRM00-T04.

## PRM00-T04 — API/dataset/verification ownership

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS

RED: TestPremiumCurrentAndPlannedContracts failed for absent /v1/premium/status. GREEN: four additive subscription paths in v1, strict no-client-account/no-premium-flag golden requests, 15 explicitly planned future dataset routes and typed schemas. Premium financial units preserve existing L/M3 vocabulary and existing unit-price bound. Owner session plus bound signed proof, private no-store/errors, generations/receipts, module/schema/job ownership and actual verification selectors are frozen in PREMIUM_API_CONTRACT.

Commands: go test -race -count=1 ./internal/platform/apicontract (11.515s); go vet ./internal/platform/apicontract; GOTOOLCHAIN=go1.27.1 staticcheck ./internal/platform/apicontract; vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check. Passed, OpenAPI zero errors (existing/new informational warnings retained). Scoped secret/whitespace checks. G-PR-CONTRACT is source-only; production capacity/catalog/provider proofs remain deferred. Next PRM01-T01.
