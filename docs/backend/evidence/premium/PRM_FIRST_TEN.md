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

## PRM01-T05 — Canonical lifecycle reconciliation and retired chains

Status: LOCAL_DONE / INTEGRATION_PENDING (source only)
Validation: PASS

RED: a pending replacement prematurely retired its paid predecessor. GREEN: only a currently eligible paid replacement retires the previous token; pending/unknown replacement preserves existing paid-through rights. Durable refresh uses the stable worker job UUID as its operation ID, canonical provider lookup, generation fences and current owner proof. Linked tokens cannot cross owners or regain rights; canonical absence of an already-bound token revokes and destroys its provider bytes. Acknowledgement is monotonic and missing-token acknowledgement queues a fenced canonical refresh.

Real PostGIS/dispatcher proofs: pending→active replacement, old-token restore denial, notification→actual refresh handler, revocation after a renewal lookup invalidates the late generation, later stale/active response cannot reinstate a revoked chain, provider outage cannot refresh rights, and retained-data access remains independent of subscription. Strict digest-only job payloads reject unknown/raw-token fields. Existing worker registers both acknowledgement and refresh with provider ports still nil.

Commands: affected integration/race suite (6.024s), all subscription unit/race tests, go vet/staticcheck, sqlc vet/diff and worker build to /tmp. Passed. Recurring reconciliation/operator runbook and real provider proof remain PRM01-T07, outside this batch and user-deferred. Next PRM01-T06.

## PRM01-T06 — Private status, purchase restore and free exit API

Status: LOCAL_DONE / INTEGRATION_PENDING (source only)
Validation: PASS
PR: https://github.com/AlexandreZanata/Abastevo/pull/131

Owner status resolves the account from the existing live session and includes a fixed official Play management destination. Verify/restore require the existing exact-body P256 request signature and the same account's contributor binding; the store rechecks account/session/key state atomically. Restore never transfers purchase ownership or recovers session/key authority. Free export pages owned retained subscription metadata after expiry without a provider, token key or paid capability. Default 20 / maximum 100, owner-bound HMAC cursor (15 minutes), strict JSON/body bounds, no token bytes/digests in export items and no-store/no ETag. Existing free deletion synchronously destroys provider token bytes and fences access.

RED/GREEN: a private status response initially inherited an unnecessary ETag from the shared public writer; the subscription writer now emits private JSON without an ETag. Real SQL/application proofs cover same-owner restore, exact/divergent-kind replay, foreign restore refusal before provider lookup, revoked-session refusal, expired export pagination/ownership and immediate provider-byte deletion. Actual API subprocess proofs use real account-key login, signed contributor requests, SQL stores and HTTP: payments-disabled status, no-store/no ETag, expired/foreign export, unsigned/cross-account/replayed-nonce restore refusal, disabled provider refusal and free deletion. No S3/provider/Android runtime certification is inferred from this bounded matrix.

Final commands (exclusive disposable loopback PostGIS; no shared/staging database):
- go test -race -count=1 ./internal/modules/subscription/... — PASS.
- go test -race -count=1 ./internal/platform/apicontract ./internal/modules/subscription/adapters/http — PASS.
- ANPFUEL_TEST_DATABASE_URL=(disposable DSN) go test -race -count=1 -p=1 -tags=integration ./internal/modules/subscription/adapters ./internal/modules/account/adapters ./internal/platform/migrate ./cmd/api — PASS (32.494s / 44.266s / 8.743s / 8.845s).
- make quick-verify — PASS, full selection, 43s: all backend build/unit/vet/staticcheck, sqlc vet/drift, OpenAPI (zero errors; 97 warnings / 51 informs retained), secret surfaces and govulncheck (no vulnerabilities).
- git diff --check and final staged secret-surface review — PASS.

Tested source anchor: candidate on parent 44653cd; SHA-256 over sorted backend/contracts paths and file SHA-256 bytes: be28a3899c16c2fca32f42582ea0393d4c05d8b7adc010d245eed15725dabca2. Only acceptance documentation changed after these final checks. The PRM01-T06 commit carrying this evidence contains that tested source.

## First-ten batch checkpoint

PRM00-T01..T04 and PRM01-T01..T06 are source LOCAL_DONE: ten task commits, plus one separately authorized notification-planning commit. PR131 targets main from codex/premium-backend; primary dev/mobile work was not edited or imported. This is INTEGRATION_PENDING until protected review/CI/merge; no merge/deploy/wiki publication was requested.

Real payments, provider authentication/catalog/OAuth/key lifecycle wiring, real Play sandbox and recurring reconciliation/operator runbook remain PROOF_OWED / PRM01-T07 by explicit owner deferral. Sales stay OFF; no runtime fake or manual entitlement grant exists. Cloud records/sync/backup/professional ledger/reports/advanced alerts/minimal Android remain their later planned tasks. OneSignal is content/phase planning only (PRM11–PRM14), with no SDK, credentials or sends. G-PR-BACKEND, commercial readiness and production certification remain INCOMPLETE.
