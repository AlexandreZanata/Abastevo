# Parallel Stripe/Asaas payment implementation plan

Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED / LOCAL_ONLY.
Date: 2026-10-24. Tasks: PAY-00–PAY-23, all PLANNED.
Rules: [B-BR-PAY-01–24 and BUC-PAY-01–10](../product/STRIPE_ASAAS_PAYMENT_CONTRACT.md).
Sources: [provider register](STRIPE_ASAAS_PROVIDER_REGISTER.md).
Executable work breakdown: [phase registry](STRIPE_ASAAS_PAYMENT_PHASES.json).
Agent handoff: [implementation prompt](STRIPE_ASAAS_PAYMENT_PROMPT.md).

## Human selection and scope

The maintainer requested a parallel planning track for Stripe card and Asaas
Pix payments for Premium and Enterprise, preparing a working backend for later
provider configuration, with Google Play as a future alternative. In an explicit
follow-up, both conventional Pix and Pix Automático were selected with independent
activation. The recommended implementation is the existing Go/PostgreSQL modular
backend; no Java service, broker or new always-on financial service is selected.

This turn produces local planning/contract/registry/prompt documentation only.
It creates no charge/refund/mandate, provider account, credential, webhook,
runtime code, dependency, migration, PR or deployment. Inserting credentials
alone is not acceptance: exact merchant/catalog/callback configuration, account
eligibility and real sandbox proofs remain required. Never promise all possible
abuse has been eliminated; track a versioned threat matrix and residual risks.

Selected future construction may progress alongside MAP and ESP in an exclusive
payment worktree. Enterprise selection covers billing and an explicitly defined
single-payer service/capability target. It does not reopen Stage D receipts,
station representation/management, team seats, a tenant model or a private portal.
No Enterprise offer can be sold until its deliverable/capabilities are defined
and implemented. Existing app-first/safety and G24/G09/commercial gates remain;
web backend readiness does not permit unreviewed Android purchase steering.

## Inspected baseline and mandatory reuse

Planning baseline is `b1a2bde6` in an isolated `stripe-asaas-plan` worktree, whose
parent source was local dev `5c9f5204`. Local/cached remote refs were inspected,
not fetched or synchronized. PAY-00 must inventory actual latest dev/main,
active MAP/ESP/PRM work and deployed revision before selecting implementation.
The portal planning commit is an ancestor, not a payment prerequisite.

- `backend/internal/modules/subscription/` already has domain lifecycle, owner-
  bound persistence, private HTTP, token protection, notification/reconciliation
  jobs and a Play adapter. Preserve accepted tests and ports; do not restart PRM.
- `application/service.go` currently treats configuration as Play provider/token/
  account-binding/package readiness and removes capabilities when unconfigured.
  `domain/policy.go` uses a 15-minute proof-freshness rule and fixed capabilities.
  New web grants cannot be disguised as Play purchase tokens or require Google
  credentials. Design provider-neutral access and source-specific verification
  freshness explicitly, with versioned policy and consumer regression tests.
- `adapters/capability.go` participates in caller transactions. Reuse this owner
  boundary for atomic access checks; do not trust a separate unchecked cache or
  add an HTTP capability check while holding a business transaction lock.
- Actual API/worker composition keeps payment provider ports nil. Existing source
  contracts/tests are not evidence of wired Stripe/Asaas or enabled real charging.
- Account/Identity owns caller proof. Existing Jobs owns durable dispatch; its
  safety loops retain priority. Existing user expense/vehicle ledgers are not the
  merchant's billing journal. No fuel payment wallet, outgoing Pix or marketplace.

## Concurrent execution and shared ownership

Before PAY-00 code: inventory worktrees/branches and modified/untracked files.
Use a dedicated payment worktree/branch (suggested `codex/payments-stripe-asaas`)
from reconciled maintained dev under ADR-020. Do not switch/move an occupied dev,
stash/reset others' edits, rebase published history or overwrite a map checkout.
Normal final integration is coordinated and preserves commits.

Payment-owned proposed areas: payment-specific domain/application/provider
adapters, fixtures, evidence under `docs/backend/evidence/payments/`, tests and
configuration proposal. Map-owned areas include CoMaps/JNI, Android map/charging,
geography/Directory/Official/Rust source preparation. ESP owns landing consultation
and public exports. This batch has no reason to edit those product areas.

Shared files require serialized ownership: subscription access ports, API/worker
composition, schema/sqlc/migration numbers, OpenAPI, Go dependencies/lock files,
CI, Caddy/Compose, ROADMAP and PROGRESS. At phase opening declare exact shared
paths in the phase evidence and verify the other working trees. Before mutation
reserve the path with the project's actual coordination mechanism or human
coordination. A clean file, an agent note or a worktree alone is not a lock. Do
not invent a lock that other agents are not using. If coordination is absent,
develop independent pure/provider code and leave the shared mutation pending.

Allocate migration/ADR sequences only at a coordinated current-head boundary;
never edit applied migrations or arbitrarily renumber published ones. Verify
semantic compatibility in addition to clean Git merge: consumer auth, enums,
routes, schema, feature switches and worker composition all matter. Never merge
unfinished MAP/ESP work merely to finish payment delivery. At final integration,
unaccepted work already in dev blocks that cumulative merge until coordinated.

Keep the detailed phase record separate and PROGRESS concise/additively merged.
One phase at a time, one meaningful commit including tests/evidence per accepted
bounded phase. If too broad, define ordered letter-suffixed subphases before
implementation; one commit per accepted child, retaining full parent obligations.
Do not spawn agents or run multiple PAY phases in parallel.

## Architecture and financial lifecycle

Separate existing-subscription/access ownership from new provider orchestration:

Billing BRL uses integer cents and exact decimal serialization/parsing (for
example JSON number text), never float64 arithmetic. Do not silently round the
fuel domain's milli-BRL values into billing cents or accept unsupported precision.

1. An authenticated account requests an admitted offer/term/method.
2. Server freezes a quote/order with exact BRL minor units and catalog version.
3. A transaction records durable creation intent and local operation identity.
4. A bounded provider adapter creates/reuses hosted Checkout or Pix resources
   outside DB locks. Ambiguous outcomes become recoverable unknown operations.
5. Authenticated callback intake persists a minimal durable inbox before success.
6. A worker verifies remote state and resource-to-order/customer/merchant binding.
7. One DB transaction records financial effects, period grant and durable follow-up.
8. Owner status reads local projection; periodic bounded reconciliation repairs
   absent callbacks, remote changes and lost replies without per-read provider I/O.

Logical entities, not proposed table names to copy blindly: CatalogOfferVersion,
BillingCustomer, Order/Quote, ProviderAttempt, BillingContract/Cycle,
PixAuthorization, FinancialFact/Refund/Dispute, Inbox/Outbox and AccessGrant.
Use explicit interfaces; keep Account and subscription as canonical owners. A
new bounded payments module is justified only for actual web order/provider
orchestration; it must not duplicate authentication, subscription or user ledgers.

Financial identifiers are scoped by provider + merchant + environment + object
type. Orders/contracts are bound to authenticated account and target. Customer
email/CPF/CNPJ cannot merge app accounts or grant enterprise administrator rights.
Every immutable economic effect has an original obligation and a unique effect
key. Internal at-least-once jobs produce one eligible effect; external uncertainty
and residual duplicate-resource risk need reconciliation, not an exactly-once claim.

### Card, conventional Pix and automatic Pix

- **Stripe:** hosted Checkout for admitted card subscriptions (and explicit
  prepaid offers only if selected); 3DS/SCA/action-required and delayed outcomes
  remain pending. Hosted customer management is owner-bound, short-lived and
  constrained to configured operations. Arbitrary portal changes cannot bypass
  offer catalog or cross-provider subscription fences.
- **Conventional Pix:** create exact charge/QR, expose private status/expiry and
  assign one policy-defined paid period only on authoritative receipt. No implicit
  recurring consent, no static QR shared among orders, no screenshot proof.
- **Pix Automático:** track payer consent/mandate separately from initial receipt;
  support activation/refusal/cancel/expiry/ineligibility and scheduled instruction
  lifecycle. Select one mode of cycle generation. Provider SUBSCRIPTION is the
  recommended starting candidate if admitted; MANUAL uses application scheduling
  only under documented provider time/retry rules. These modes are not the same
  thing as conventional Pix. Both selected public methods get independent flags.

Manual renewal, month/year anchors, early/late payments, paid-through cancellation,
upgrades/downgrades, price changes, prorations, partial refunds and disputes need
golden examples at PAY-01/03. Recommended initial change policy: changes at the
next term boundary, no implicit proration; immediate upgrades/proration remain
explicitly unsupported until separately specified/tested. Do not label an endpoint
as supporting every change if it safely refuses unsupported transitions.

One recurring funding source owns a target/product-family contract. Method
switching first safely fences/cancels the old renewal source, then activates the
new source under explicit consent, preserving already earned periods. Unknown
remote cancellation prevents a new automatic debit. Concurrent manual payment
and automatic settlement may create excess money, but not duplicate access;
record and reconcile the excess under the approved compensation policy.

Refund requests reserve the refundable amount atomically, tolerate lost replies,
and finalize only from authoritative state. Disputes/MED affect the matched
payment's policy-defined rights; unknown account-level holds trigger operator
review rather than revoking every customer. Operator refund/replay tools require
least privilege, fresh strong proof, reason/audit and amount-dependent approval
policy. No arbitrary outgoing bank destination or public manual grant endpoint.

### Merchant catalog and privacy decisions before sale

PAY-01 freezes nonsecret offer configuration: PREMIUM/ENTERPRISE identifiers,
actual deliverables/capabilities, supported terms, BRL amounts, taxes/invoice
handling, price version, provider mappings, recurring consent text, grace and
exit/retention, refund/partial allocation, duplicate/late payment handling and
allowed channels. Missing prices/features do not block generic code using clearly
synthetic fixtures; they do block real checkout and commercial readiness.
PAY-01 is LOCAL_DONE: [offer and lifecycle policy v1](../product/STRIPE_ASAAS_OFFER_POLICY.md)
freezes the technical envelope and golden fixture, leaves production prices/
Enterprise/fiscal/legal inputs unresolved and keeps all three channels disabled.
PAY-02 is LOCAL_DONE: [provider admission v1](STRIPE_ASAAS_PROVIDER_ADMISSION.md)
pins Stripe Go `v87.0.0`/API `2026-09-30.endive` and the Asaas v3 client approach,
with synthetic contract fixtures. No dependency/runtime/account change; SDK
installation remains at PAY-06 and account/sandbox proof remains PAY-21.
PAY-03 is LOCAL_DONE: `backend/internal/modules/payments/domain` now owns pure
order/attempt/settlement/contract/mandate/refund/dispute/grant transitions,
exact BRL periods and source allocation. Existing Play/free exits are unchanged;
persistence/providers/runtime remain PAY-04+.
PAY-04 is LOCAL_DONE: migration 000123 and the payment sqlc/store layer persist
idempotent operations, attempts, settlements/grants, cycles, inbox and job_queue
outbox with lease/generation fences. Provider/runtime work remains PAY-05+.
PAY-05 is LOCAL_DONE: provider mode/merchant/catalog/key/return-origin config,
independent new-sales and maintenance switches, and a redacted read-only
preflight are validated; missing/mixed values disable only unsafe sales.
PAY-06 is LOCAL_DONE: server-catalog Stripe hosted Checkout, owner-bound customer
mapping, stable idempotency, durable ambiguous recovery and card-only/return-
origin checks are implemented and tested; runtime/webhook wiring remains PAY-08+.
PAY-07 is LOCAL_DONE: conventional Asaas Pix uses owner customer mapping, exact
server amount/due date/external reference, bounded owner QR/status, serialized
creation and external-reference recovery; no auto-debit/settlement is added.
PAY-08 is LOCAL_DONE: Stripe webhooks use exact raw-body SDK verification, mode/
version/account binding, bounded input and a durable inbox before HTTP 200;
unknown/duplicate/conflicting events are safe and runtime wiring remains PAY-19.
PAY-09 is LOCAL_DONE: Asaas webhooks use the separate constant-time token with
rotation, TLS/bounds/account binding and durable inbox before HTTP 200; unknown/
duplicate/conflicting delivery and DB retry are safe.
PAY-10 is LOCAL_DONE: canonical Stripe/Asaas settlement verifiers, atomic
settlement/grant/cycle/outbox processing, revocation/replay fences and web-grant
subscription capability/status projection are implemented; no Play token or
credential is required for web-funded access.
PAY-11 is LOCAL_DONE: Stripe provider subscriptions are persisted separately from
invoice-funded grants, lifecycle/SCA/past-due/cancel/reconcile and constrained
owner portal are implemented; cancellation preserves only paid-through and
invoice creation grants nothing.
PAY-12 is LOCAL_DONE: Pix Automático consent/authorization, mandate persistence,
initial paid-period independence, refusal/expiry/cancel and lost-reply recovery
are implemented; merchant eligibility remains an external gate and auto Pix stays
disabled until proof.
PAY-13 is LOCAL_DONE: provider-owned Pix Automático instructions, retry policy,
received-cycle settlement, durable cancellation and eligibility blocks are
implemented and reconciled under concurrency; the application never creates a
second charge owner. Provider proof remains PAY-21 and runtime wiring remains
PAY-19.
PAY-14 is LOCAL_DONE: next-term plan/price changes and cross-provider switches
are durably fenced; the old card/Pix Automático source must be confirmed stopped
before a new source may start, unknown cancellation blocks activation, and
immediate/prorated changes are refused. Provider proof and runtime wiring remain
PAY-21/PAY-19.
PAY-15 is LOCAL_DONE: owner cancel/status/history/export use a durable,
proof-protected billing-exit intent; account deletion records cancellation before
mutation, retains minimal financial records, never refunds automatically and
leaves uncertain provider cancellation visible. Runtime worker/routes remain
PAY-19.
PAY-16 is LOCAL_DONE: refunds execute only on the verified original route under
entitlement-locked reservation with operator proof/reason/audit and dual approval
above the limit; disputes revoke only matched unexpired paid periods; unmatched
money becomes an explicit hold for review; stale replays and concurrent refunds
cannot duplicate provider writes. Runtime wiring and provider proof remain
PAY-19/PAY-21.
PAY-17 is LOCAL_DONE: enterprise billing binds one exact approved offer to a
single accountable payer and its own account target, with no app capabilities
and no team/station/organization privilege; unapproved offers are refused on
every channel without side effects; provider invoice/receipt references are
owner-scoped; the fiscal engine stays explicitly frozen with no tax construction.

No default fabricated price or Enterprise seats. No coupons/trials/zero-price
grants/installments/wallet credit unless explicitly specified. Distinguish provider
billing invoice/receipt from legally required fiscal issuance; record responsible
issuer/workflow without building an unrequested tax engine. Freeze mandatory
billing data and legal retention with the owner; avoid unnecessary CPF/CNPJ.
Customer notification delivery/costs are configured explicitly, not automatically
duplicated through provider email/SMS/WhatsApp and application notifications.

## Efficiency, configuration and operational controls

Use the existing PostgreSQL inbox/outbox/jobs with indexed bounded queues,
per-provider concurrency, connection/time/response-size limits, exponential
backoff/jitter and attempt/dead-letter budgets. Protect safety work and isolate
payment budgets from import/map builds. Store provider state locally with a
versioned maximum freshness/staleness policy; a provider outage neither creates
new unverified rights nor silently destroys otherwise valid paid-through periods.

Webhook intake is authentication + durable bounded insert + response. Stripe
uses official SDK raw-body verification; Asaas requires its distinct webhook
token. Reply HTTP 200 for a durably recorded Asaas event, including recognized
duplicates. Never acknowledge storage failure or place remote verification on
the callback latency path. Accept additive unknown fields safely; unknown event
types cause no grants. Persist only minimal selected data or encrypted short-
retained originals when required; no full unbounded PII payload archive.

Per-channel enablement, test/live mode, merchant identity, catalog mapping,
origins, API version, webhook events/keys, encryption/rotation and operational
limits are validated on startup by a read-only preflight. Missing or mixed
configuration fails closed for that sale channel with safe diagnostic codes.
Readiness distinguishes technical liveness, channel/configuration validity,
provider connectivity, queue health and commercial approval. No secrets in
responses/build output, no startup creation of real products/charges or hidden
enablement when a key happens to be present.

Separate `new_sales_enabled` from receipt/reconciliation/cancel/refund processing.
An emergency sales switch must not strand existing subscriptions. Revoked
provider credentials require alerts/recovery, not fake successful cancellation.
Dashboard/account setup is documented; an explicit idempotent provisioning
command may be built for approved test configuration, with dry-run and ownership
checks. Never automatically rewrite unrelated provider catalog/webhooks.

Required operational signals: checkout/admission/error rates, provider latency/
429/outage, ambiguous writes, financial/access mismatch census, oldest inbox/job,
dead letters, webhook rejection/retries, Asaas queue interruption, mandate
eligibility, pending refunds, unknown disputes and reconciliation last success.
Logs are correlation IDs/codes/counts only, excluding payment URLs/QRs, tokens,
CPF/CNPJ/customer email/raw signatures. Key rotation allows tested overlap and
rollback without disabling authentication or dropping historical work.

## Ordered phases and gates

The [JSON registry](STRIPE_ASAAS_PAYMENT_PHASES.json) is the detailed checklist
for a less capable agent: dependencies, owned/shared areas, steps, meaningful
tests, evidence and commit topic. All tasks are PLANNED. Introduce exact real
test commands at phase opening from current code; registry prose is not a claim
that a nonexistent command or endpoint is available.

- **PAY-00:** actual baseline, parallel ownership, existing PRM reuse and risk inventory.
- **PAY-01:** Premium/Enterprise catalog, lifecycle/period/refund policy and channel gates.
- **PAY-02:** provider/API/SDK/source admission and dependency/license review.
- **PAY-03:** pure order/contract/settlement/grant state machines and golden cases.
- **PAY-04:** additive persistence, intent, inbox/outbox and concurrency fencing.
- **PAY-05:** credential boundaries, per-channel configuration and read-only preflight.
- **PAY-06:** Stripe card customer/Checkout creation and uncertain-write recovery.
- **PAY-07:** Asaas conventional Pix charge/QR and uncertain-write recovery.
- **PAY-08:** authenticated Stripe durable event intake.
- **PAY-09:** authenticated Asaas durable event intake.
- **PAY-10:** canonical verification, financial effects and existing access integration.
- **PAY-11:** Stripe renewals, failures and constrained hosted management.
- **PAY-12:** Pix Automático initial payment/consent/mandate lifecycle.
- **PAY-13:** Pix Automático cycle generation/retries/cancellation and eligibility.
  LOCAL_DONE; provider-owned cycles, instructions, eligibility and durable
  cancellation tested.
- **PAY-14:** plan/term changes and cross-provider funding-source fences.
  LOCAL_DONE; next-term changes, confirmed old-source cancellation, stale-quote
  and concurrency fences tested.
- **PAY-15:** cancellation, account exit/deletion and recovery. LOCAL_DONE;
  proof-protected cancel/status/export, durable delete-before-mutation intent and
  retained billing tombstone tested.
- **PAY-16:** full/partial refunds, disputes/MED and restricted operator recovery.
  LOCAL_DONE; bounded reservation, canonical verification, revocation rules and
  operator fences tested.
- **PAY-17:** Enterprise single-payer billing and capability-target isolation.
  LOCAL_DONE; binding policy, cross-channel refusal, approved self-purchase and
  no-capability grant isolation tested; fiscal workflow frozen.
- **PAY-18:** adversarial matrix, privacy/restore and bounded performance acceptance.
- **PAY-19:** actual API/worker composition and regressions with MAP/ESP/PRM.
- **PAY-20:** deployment/configuration packaging, preflight and operations runbook.
- **PAY-21:** actual Stripe test-mode and Asaas sandbox end-to-end acceptance.
- **PAY-22:** final coordinated dev integration, protected PR/CI and guarded merge.
- **PAY-23:** scoped VPS test-mode deployment and explicit production handoff.

Local code/data/API work through PAY-20 can finish without provider credentials
using test-only fixtures and a real disposable PostGIS runtime. Mock fixtures
must never be deployable grant/payment bypasses. PAY-21 cannot pass without
actual external test accounts/configuration. Unsupported Pix Automático sandbox
cases stay explicitly PROOF_OWED; do not simulate them in production or remove
the requirement. Independent completed work remains a truthful local checkpoint.

Readiness states:

- **PAY-LOCAL_READY:** selected source code, exact contract/risk/SQL/concurrency/
  process tests and disabled-by-default deployment package pass at a pinned tree.
- **PAY-CONFIG_READY:** every required secret/nonsecret field is documented and
  preflight rejects incomplete/mixed configurations; no real-provider proof implied.
- **PAY-SANDBOX_ACCEPTED:** actual merchant test configuration and complete real
  card/Pix/manual/automatic journeys pass for the selected channel independently.
- **PAY-INTEGRATED:** exact current protected PR head/base checks and merge pass.
- **PAY-VPS_TEST_ACCEPTED:** pinned integrated artifact works over actual HTTPS
  in the authorized test environment, including real callbacks and recovery.
- **PAY-COMMERCIAL_READY:** actual live account eligibility/catalog/terms/channel
  policy/operational approval and separately authorized live-money acceptance
  pass. No real charge or refund is authorized by the current planning turn.

If sandbox is unavailable, the code may be reviewed as provider-proof-pending
only with that explicit scope; do not label the full payment batch finished or
auto-activate live checkout. Never silently bypass existing G11/G24/G09 gates.

## Security and acceptance matrix

PAY-00 records test IDs/owner/status/residual risk for each category below;
PAY-18 closes implemented cases and PAY-21 adds provider proof. Required cases:

- Auth: forged/revoked/foreign account/session/key; IDOR for order/customer/QR/
  invoice/portal/cancel/refund; CSRF on cookie-bound browser commands; replay.
- Catalog: modified amount/currency/term/quantity/offer; changed catalog after
  quote; rounding/overflow/negative/zero; expired quote; unimplemented Enterprise.
- Remote writes: retries before/after remote dedupe window; provider created but
  reply lost; webhook before response; response lost after DB commit; two workers;
  stale lease; event/refund duplicate; no proven remote absence after timeout.
- Webhooks: missing/bad/old/rotated signature or token; raw body changed; wrong
  merchant/live mode/version/resource; huge/malformed JSON; duplicate ID with
  altered content; additive field/unknown event; DB outage and flood/backpressure.
- Settlement: pending/authorized versus captured/received; manual paid invoice;
  provider/customer/order mismatch; late/refunded payment; duplicated manual and
  recurring cycle; independent failed mandate and successful initial Pix payment.
- Lifecycle: SCA/3DS/action-required/decline; month-end/leap year; grace/past-due;
  renewal/cancel/refund races; expired QR; Pix refusal/retry/eligibility/cancel;
  provider switch with unknown old cancellation; operator plan change in dashboard.
- Recovery: missed/paused webhook queue, dead job, provider outage/429, deleted
  account, revoked key, database restore and stale outbox, refund insufficient
  funds, chargeback/MED reversal and hold without associated payment.
- Boundaries: no card data in application; no PII/secrets in logs/artifacts;
  protected billing retention versus erasure; no paid trust; no free-data loss;
  no cross-account/Enterprise rights; no Android/map/static-portal regression.

Freeze measurable ceilings before performance acceptance: callback DB latency,
order/owner query plans, throughput versus admitted scenario, provider call rate,
CPU/RSS/connections, inbox/outbox age/recovery time and financial-consistency
denominators. Measure accepted/denied/failed operations separately and count
actual external resources; fast duplicate charges are a failure, not throughput.
Use remote sandbox within quotas and fault fixtures for load, never live-money
stress or load against provider public endpoints. No universal latency numbers
are invented before baseline; 16 GB RAM is not a capacity promise.

## Final delivery and real operation

Prepare scoped artifacts/configuration and test them before any required final
approval. Use the existing workflow: local checkpoints, no per-phase PR/CI wait,
final cumulative protected dev → main with actual required Quick verification
and applicable checks, guarded finish and retained dev. Do not merge unrelated
unfinished MAP/ESP branches or push main directly. Changed head/base invalidates
affected evidence; repair with new commits. Wiki follows its existing owned scope.

PAY-22's phase commit freezes useful reconciliation changes and pre-merge
acceptance evidence before the PR head is finalized. Store actual merge/check
identifiers in PR metadata or the next useful authorized commit; do not create
a recursive post-merge bookkeeping-only commit/PR to record its own acceptance.

PAY-23 requires named VPS/environment/host ownership, least-privilege process
secrets, backup/recovery and a coordinated deployment window. Default first
deployment is provider TEST/SANDBOX, real checkout still off. Verify actual HTTPS
callbacks, account isolation, order → payment → access → renewal/cancel/refund,
worker recovery and existing app/map/public-static smoke. Use real sandbox events,
not forged live payment notifications. Isolate simulated outages; do not stop
shared production APIs. A Caddy/Compose diff touching shared infrastructure needs
its specific operational scope; no other apps, global prune or arbitrary restart.

Production activation is channel-by-channel after external gates and explicit
live-money scope. Configurations alone do not satisfy unknown merchant approval,
Pix Automático eligibility or app-store policy. Inserting secrets should require
no code edits after acceptance, but still requires the verified configuration/
enablement procedure and actual external proofs. The handoff must name remaining
owner actions precisely instead of claiming “just add keys and everything works”.

## Validation of this planning change

Documentation-only validation: local links/anchors, 24 unique sequential phase
IDs and dependency DAG, contract/source/phase consistency, scoped whitespace and
sensitive-surface review. No backend/provider/runtime/capacity test was executed
as a consequence of creating this plan. Record actual check results in PROGRESS.

Executed 2026-10-24: one-off Python 3 checks passed 28 new/changed local links/
anchors, 24 ordered registry/roadmap/plan entries and dependency DAG, 24 rules,
10 use cases, three selected channels, final-newline/whitespace and credential-
pattern checks. The 23 external source links were counted structurally, not
certified as live account/API contracts. `git diff --check` passed. Actual provider
research limitations remain in the admission register; no runtime tests ran.

Future runtime commands must come from [backend README](../../backend/README.md),
[test strategy](../backend/TEST_STRATEGY.md), current source and [CI cadence](CI_PLAN.md).
Existing examples from backend/: `go test -race ./internal/modules/subscription/...`
and the corresponding `-tags=integration` suite after disposable DB setup. New
payment-package commands require real package/test creation, not guessed selectors.
Run format/vet/sqlc/contract/migration/negative tests appropriate to actual changes,
then `git diff --check`. Use the pinned project toolchain and review new SDK
dependencies before installation. This request changes no runtime configuration.
