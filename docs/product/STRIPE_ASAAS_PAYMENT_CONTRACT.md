# Stripe and Asaas payment contract

Status: PROPOSED / PLANNING_ONLY. Date: 2026-10-24.
Owner: [PAY-00–PAY-23 plan](../planning/STRIPE_ASAAS_PAYMENT_PLAN.md).
Sources: [provider admission register](../planning/STRIPE_ASAAS_PROVIDER_REGISTER.md).

## Selected scope and product boundary

The maintainer selected Stripe for cards, Asaas for Pix, Premium and Enterprise
billing, and Google Play as a later alternative. The maintainer explicitly
selected BOTH conventional Pix payment per period and Pix Automático, with
independent activation. Implement with the current Go/PostgreSQL architecture;
no Java service or payment processor is built in this planning change.

This request creates a parallel implementation plan and agent prompt. It does
not itself create a charge, enroll an account, configure a provider, expose a
webhook, deploy code, collect documents or initiate a real-money refund.
The implementation prompt selects the future local construction scope.

Enterprise scope is billing a defined single accountable payer for a versioned
offer, mapped to an explicitly authorized account/service target. It is not
multi-tenancy, corporate representation, station management, team/seat access
or permission to sell unimplemented capabilities. Those Stage D/product slices
remain separately governed. Provider business-customer identity is not proof of
representation, organization membership or ownership of a station.

The portal consultation remains static and public. Financial APIs/webhooks are
separate authenticated/dynamic boundaries. No payment API or secret is embedded
in generated public datasets. Android purchase links/UI remain outside this
batch until applicable Play distribution/program requirements are verified.

## B-BR-PAY-01–24

- **01 — Catalog authority:** the server selects versioned offer, amount, BRL
  minor units, term/calendar rules, eligible target, capabilities and provider
  mappings. A client cannot set price, discount, paid-through time, quantity,
  customer ID or entitlement. Reject sale of incomplete/unconfigured offers.
- **02 — Proof and binding:** payment commands and private reads require the
  existing live account/session/key proof. Bind order/customer/payment/target
  explicitly. Email, CPF/CNPJ, provider metadata or a public order ID alone
  never authenticates a caller or transfers access to another account.
- **03 — Distinct state machines:** order, remote attempt, settlement, recurring
  contract/mandate, refund/dispute and capability grant are separate. Store
  original provider statuses with versioned mappings; unknown states cannot
  grant access. A completed redirect or created invoice is not settlement.
- **04 — Paid evidence:** grants require authoritative matching merchant,
  environment, resource, order/customer, currency, amount, offer and eligible
  payment outcome. Stripe invoices manually marked paid/out-of-band do not
  authorize a card grant without the approved verified settlement policy.
  Trials, zero invoices, discounts, balances and installments stay disabled
  unless a catalog/policy explicitly admits them with separate proof rules.
- **05 — Stripe custody:** use hosted Checkout and approved hosted management
  flows, with cards as the selected payment method. Never accept/store/log PAN,
  CVV or raw card forms. Pin SDK/API and webhook versions; use maintained
  verification/crypto libraries, not a handwritten payment SDK or signature.
- **06 — Pix custody:** create an owner-bound charge and expose only the
  allowed QR/copy-paste data and real expiry. Screenshot/receipt upload, client
  confirmation, authorization creation and payment-created notifications do
  not prove payment. Define late, under/overpayment and duplicate settlement.
- **07 — Two Pix modes:** conventional Pix has no implicit auto-debit consent.
  Pix Automático requires admitted merchant eligibility and active payer
  authorization for future cycles. Initial settlement and mandate activation
  are independent; authorization refusal can coexist with a paid first period.
- **08 — One recurring scheduler:** a recurring contract has one documented
  charge-creation owner, provider or application, never both. Reuse a failed
  cycle's supported retry process rather than creating another charge/mandate.
  Revoked/expired/ineligible mandates cannot create new instructions.
- **09 — Local idempotency:** stable operation key + payload hash + owner +
  environment/merchant scopes protect requests for the full retention policy.
  Same key/different request conflicts. Unique order/cycle/remote-effect and
  grant constraints stop duplicate internal effects under concurrency.
- **10 — Ambiguous remote writes:** persist intent before dispatch. Stripe
  retries reuse the applicable key/parameters within verified provider rules.
  Asaas correlation is not assumed to be provider-enforced idempotency.
  A timeout means OUTCOME_UNKNOWN: reconcile before resending. If absence cannot
  be proved safely, quarantine for operator resolution; never blindly recreate
  a charge, customer, authorization, subscription or refund. Do not claim
  mathematically exactly-once external writes without provider guarantees.
- **11 — Authentic webhook intake:** Stripe verification uses bounded exact
  raw bytes, signature and timestamp tolerance with trusted time. Asaas uses
  mandatory separate webhook authToken in asaas-access-token, compared safely
  over TLS; do not invent a Stripe-like Asaas HMAC signature. Treat leaked
  bearer-token risk with rotation and authoritative resource verification.
- **12 — Durable acknowledgement:** authenticate/validate bounds, persist an
  inbox entry atomically, then acknowledge. Asaas success is HTTP 200. Duplicate
  IDs have one durable effect; changed content for an existing identity is
  recorded as a conflict, not reprocessed. Unavailable storage receives no fake
  success. Unknown authenticated events are retained minimally for review and
  cause no financial effect. No provider calls in the webhook response path.
- **13 — Reconciliation and ordering:** process bounded jobs from durable
  inbox/outbox, refresh canonical provider resources and fence stale commits.
  Repeated, late and reordered events cannot extend/refund twice or resurrect
  revoked grants. Durable cursor/overlap/census reconciliation covers lost
  webhooks and paused queues without polling on each app read.
- **14 — Period and access:** exactly one contribution per paid obligation,
  with immutable start/end and policy version. Manual renewal, early payment,
  late settlement, leap dates/time zones and month-end anchors have explicit
  rules. Cancellation stops future renewal; verified earned access lasts until
  its policy end unless separately revoked. Preserve free access and local data.
- **15 — Cross-provider safety:** one recurring funding source per target/
  product-family contract. Switching or replacing checkout cannot leave two
  autonomous debit mandates active. Fence the old source before enabling the
  new one; unresolved cancellation stays pending. Concurrent Pix/card payments
  are recognized financially but cannot double-grant the same obligation.
  Handle excess/late settlements with reviewed compensation, never hide money.
- **16 — Refund and dispute:** cumulative pending/succeeded refunds are bounded
  by refundable original gross under a lock; failure/request is not completed
  refund. Refund only to the original provider payment route, never an arbitrary
  Pix key. Restrict financial actions to fresh authorized operators/owner policy,
  reason/audit and an approved step-up/approval matrix. Account deletion does
  not auto-refund. Reconcile chargebacks, Pix MED and partial/reversed disputes.
- **17 — Proportionate risk:** freeze dispute/refund effects per original
  payment and capability source. Account-level balance blocks with no matched
  payment do not revoke every payer. A dispute is not a confirmed fraud finding.
  Avoid blanket bans, account enumeration or security-through-hidden-rules.
- **18 — Capability isolation:** extend the existing subscription/access owner
  through explicit ports and provider-neutral grants; do not fake a Play token.
  Disabling future checkout must not erase settled rights or disable webhook,
  cancellation, reconciliation, export or refunds. Mission grants remain an
  independent future source; revoking one payment affects only its grant.
- **19 — Free exits and deletion:** preserve owner export/deletion and existing
  locally held data on expiry. Deletion records cancellation/recovery intent,
  removes public/private access appropriately and applies the reviewed financial
  retention/legal basis. Restores cannot reinstate deleted ownership or cause
  old outbox writes to create charges/refunds again.
- **20 — Minimal secrets and PII:** separate test/live credentials, merchants,
  webhook keys, encryption keys and namespaces. Minimize CPF/CNPJ/contact/billing
  data; encrypt retained sensitive references as required; mask logs/traces/errors.
  Define retention per category and backup/restore, not an unlimited event log.
  Billing documents differ from fuel-photo media; neither extends the other's
  retention. No real PII/keys/payment URLs/QR secrets in Git/fixtures/logs.
- **21 — Abuse and resources:** bounded per-owner/operation/merchant admission,
  global spend/charge budgets, payload/response/time limits, retry/dead-letter
  budgets and concurrency controls protect checkout and callbacks. Account/key
  churn, card testing, QR harvesting, forged amounts and notification storms
  receive meaningful tests; exact thresholds require a frozen sizing policy.
- **22 — Efficient trust boundaries:** external calls hold no long DB locks;
  local access reads avoid provider calls and have explicit freshness/validity.
  Separate financial workers and quotas from map/ingestion/safety workloads.
  Shared external payment APIs use TLS/allowlisted fixed origins, no arbitrary
  redirects/URLs; status and redirect pages are never trusted payment evidence.
- **23 — Activation:** missing secrets/catalog/approved account/webhook/capability
  mappings keep that sale channel off with a safe actionable diagnostic. Flags
  for Stripe card, manual Pix and Pix Automático are independent. No secret
  presence alone switches production charging on. Business changes, tax/fiscal
  duties, required notices and distribution policy need named decisions.
- **24 — Honest assurance:** local tests, external test-mode proof, integrated
  deployment and live-money certification are distinct gates. Adversarial coverage
  is an explicit versioned risk matrix, not a claim to defeat all possible abuse.
  No sandbox fixture or mocked provider proves a real provider account is ready.

## BUC-PAY-01–10

- **01 Catalog/checkout:** authenticated payer selects an admitted offer; server
  creates a durable owner-bound order and one bounded provider intent.
- **02 Card settlement:** Stripe hosted checkout handles card/SCA/3DS; callbacks
  and reconciliation verify settlement and atomically project the correct grant.
- **03 Conventional Pix:** issue charge/QR, expose private status, recover expiry/
  late payment, verify receipt and allocate the paid period once.
- **04 Pix Automático:** record consent context, distinguish initial payment and
  active mandate, track scheduled cycles/retries and stop future instructions
  on cancel/expiry/ineligibility without losing a valid first payment.
- **05 Renewal/change:** renew, schedule upgrade/downgrade or switch method with
  explicit price/term/consent; prevent parallel recurring debit sources.
- **06 Event/reconcile:** authenticate/persist inbox, process with at-least-once
  jobs and reconcile remote state into one versioned local effect.
- **07 Cancel/export/delete:** verify owner, stop future billing safely, preserve
  policy-approved paid-through/free exits and resume uncertain cancellations.
- **08 Refund/dispute:** authorized actor reviews and executes a bounded original-
  route refund; consume dispute/MED events and adjust only the affected rights.
- **09 Enterprise:** bind billed entity to the authorized single payer/offer;
  match invoice/payment/term without granting organization or station privileges.
- **10 Operate/recover:** rotate keys, recover missing/orphaned operations, restore
  without replayed charges, reconcile and test separately before channel activation.
