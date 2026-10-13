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

## PRM01-T01 — Pure lifecycle and capabilities

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS

RED: ACTIVE/CANCELED/GRACE could not grant the requested six capabilities. GREEN: deterministic provider state mapping, paid-through/grace, exclusive expiry/900s freshness, future/unverified proof refusal, inactive owner/retired chain denial, free held-data access and independent result slices. Typed application principal/provider/store/token-codec ports contain no client premium Boolean or community dependency. No runtime provider is wired.

Commands: go test -race -count=1 ./internal/modules/subscription/domain; go vet and GOTOOLCHAIN=go1.27.1 staticcheck on that package. Passed. Application ports compile in the next owning persistence task; no application behavior is claimed here. Signed/owner persistence follows in PRM01-T02. PR131 is draft; original dev untouched.

## PRM01-T02 — Owned durable purchase persistence

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS

RED: actual PostGIS had no subscription persistence. GREEN: append-only 000075 above known dev reservations; sqlc owner/session/key joins, per-account serialization, token verification generations, bounded pending/hourly admissions, immutable operation payloads, unique purchase ownership, encrypted provider token envelopes and atomic digest-only acknowledgement intent. Account deletion atomically destroys pending/purchased token bytes and retires grants, even without worker execution. No provider/key is configured in production.

Real PostGIS race suite passed: same-token 12-way single winner, distinct-token 12-way exactly-two pending ceiling, exact/divergent replay, foreign owner/session, revoked key/session/deleted owner during commit, stale generation refusal, injected job-insert rollback, deleted-token purge, empty/previous-schema upgrade/replay, and actual owner status EXPLAIN over 10k foreign purchases uses subscription_purchases_owner_idx. Standard AES-GCM tests cover random nonce/AAD/tamper/weak-key refusal.

Commands: ANPFUEL_TEST_DATABASE_URL=(exclusive loopback disposable DSN) go test -race -count=1 -tags=integration ./internal/modules/subscription/adapters; affected go vet/staticcheck; sqlc vet/diff; scoped secret/diff check. Passed. No shared DB/VPS/mobile mutation. Next PRM01-T03.

## PRM01-T03 — Prepared canonical verification and admission

Status: LOCAL_DONE / INTEGRATION_PENDING (source only)
Validation: PASS; real provider/catalog wiring PROOF_OWED by user direction

Restricted Developer API adapter uses fixed HTTPS authority, injected OAuth credential port, copied supported product/base-plan allowlist, exact obfuscated account/environment/package binding, bounded body/time/no redirects, canonical provider states and redacted errors. HMAC account binding is separate from provider token encryption. Application admission validates signed principal, immutable operation kind/payload, fresh authoritative proof and atomic owner commit; no fake provider/credential/catalog is configured in production.

Local TLS fake-provider tests cover wrong owner/product/base/environment, unknown/pending state, bad expiry/ack, 404/429/5xx, malformed/oversize response, canceled request and refused redirect. Real PostGIS application tests prove provider HTTP holds no session locks, revocation during lookup prevents a grant, exact replay performs one provider call and denied proofs discard token bytes. Disabled-provider tests prove no persistence/grant and free data access metadata.

Commands: go test -race -count=1 ./internal/modules/subscription/...; disposable real SQL go test -race -count=1 -tags=integration ./internal/modules/subscription/adapters -run '^TestAdmission'; affected go vet/staticcheck. Passed. No new dependency, live provider call or payment activation. Next PRM01-T04.

## PRM01-T04 — Durable acknowledgement and authenticated provider inbox

Status: LOCAL_DONE / INTEGRATION_PENDING (source only)
Validation: PASS; real provider sandbox deferred by user

Append-only 000076 records bounded immutable notification receipts. RSA-2048/RS256 verification checks trusted key ID, Google issuer, exact audience/service-account/email proof and token times; provider handler authenticates before persistence, checks package/subscription/envelope bounds, drops expired candidates and stores no raw purchase token. Duplicate receipts converge; changed payload conflicts. Authenticated notices invalidate cached proof/generation and atomically enqueue canonical refresh rather than granting from notification type. Existing worker registers digest-only acknowledgement; payment ports remain nil.

Tests: actual generated RSA signatures/tamper/issuer/audience/service identity/expiry; private HTTP missing/forged/duplicate auth, wrong subscription, stale/oversize/duplicate JSON; local TLS canonical acknowledgement; actual PostGIS 12-way duplicate receipt produces one refresh job and immediately fences stale rights; real dispatcher lost acknowledgement reply recovers by canonical acknowledged lookup with one external acknowledge effect. Migration upgrade/replay remains explicit through 000076.

Commands: affected subscription unit/race and SQL integration suites, go vet/staticcheck, sqlc vet/diff and worker build to /tmp. Passed. Private provider sandbox is not claimed. Next PRM01-T05; no OneSignal implementation (that remains planning only).
