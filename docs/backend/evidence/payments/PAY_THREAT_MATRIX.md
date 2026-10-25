# Payment threat and test matrix

Version: 1 (2026-10-24, PAY-00).
Owner: PAY-00–23 payment track. Source plan:
[STRIPE_ASAAS_PAYMENT_PLAN.md](../../../planning/STRIPE_ASAAS_PAYMENT_PLAN.md).
Rules: [B-BR-PAY-01–24 and BUC-PAY-01–10](../../../product/STRIPE_ASAAS_PAYMENT_CONTRACT.md).

This matrix is the versioned acceptance checklist for payment abuse, failure and
money-consistency cases. It records obligations, not passing tests. Version 1 was
created from source inspection at PAY-00; no payment code, provider account or
provider test ran. Every phase that implements a row must update it with the
actual test/evidence location and close it only after the required local and, where
applicable, provider proof exists.

## Acceptance states

- **OPEN** — no payment-specific implementation/test exists. It must be proven at
  the named phase; a plan, mock or compile is not evidence.
- **REUSE_OPEN** — existing subscription/PRM source or historical evidence is a
  candidate to extend. PAY-00 did not re-execute it, so it is not accepted for a
  web payment source yet.
- **CLOSED_LOCAL** — actual local positive/negative/failure/concurrency test passed
  and the command/result is recorded. No row has this state at PAY-00.
- **SANDBOX_PROVEN** — the actual provider test environment proved the case. Only a
  real sandbox/test-mode journey can set this state; reserved for PAY-21.
- **LIVE_PROVEN** — real-money acceptance under separate explicit authorization.
  Not authorized by the payment plan or this phase.

Provider-proof column: `none` means local evidence is sufficient for the local
gate; `sandbox` means actual provider test proof is required; `live` means the
case cannot be fully accepted without separately authorized live verification.

## Auth, ownership and replay

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| AUTH-01 | Forged, revoked, expired or foreign account/session/key cannot create or read a payment command. | CLOSED_LOCAL | PAY-05, PAY-19 | none |
| AUTH-02 | IDOR on order, customer, QR, invoice, portal session, cancellation and refund is denied. | OPEN | PAY-06, PAY-07, PAY-11, PAY-15, PAY-16 | none |
| AUTH-03 | CSRF/cookie-bound browser commands require the existing proof; no state change from a redirect or cross-site form. | CLOSED_LOCAL | PAY-06, PAY-19 | none |
| AUTH-04 | Duplicate command/webhook delivery and replay cannot repeat a charge, grant, refund or cancellation. | OPEN | PAY-04, PAY-08, PAY-09 | none |
| AUTH-05 | Operator refund/replay requires fresh strong proof, reason/audit and approved privilege; key rotation invalidates old proof. | CLOSED_LOCAL | PAY-16, PAY-18 | none |

## Catalog, quote and price integrity

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| CAT-01 | Client-supplied amount, currency, term, quantity, customer, offer or target cannot change the server quote. | OPEN | PAY-01, PAY-03, PAY-06, PAY-07 | none |
| CAT-02 | Catalog/price/terms change after quote is detected; a stale quote cannot be silently repriced or sold. | OPEN | PAY-03, PAY-04 | none |
| CAT-03 | Rounding, overflow, negative, zero, unsupported precision and milli-BRL/cents mixing are refused or exact. | OPEN | PAY-01, PAY-03 | none |
| CAT-04 | Expired quote cannot create a provider charge or grant access. | OPEN | PAY-03, PAY-06, PAY-07 | none |
| CAT-05 | Unimplemented Enterprise service, seat/team quantity or arbitrary target is refused. | CLOSED_LOCAL | PAY-01, PAY-17 | none |

## Remote writes and ambiguous outcomes

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| RMT-01 | Retry before/after provider dedupe retention reuses or reconciles the original intent; it never blindly creates a second resource. | OPEN | PAY-04, PAY-06, PAY-07 | none |
| RMT-02 | Provider created a customer/charge/subscription but the reply was lost; recovery finds or quarantines it as OUTCOME_UNKNOWN. | OPEN | PAY-04, PAY-06, PAY-07, PAY-16 | none |
| RMT-03 | Webhook arrives before the local create response or after DB commit but before response; effects stay single and consistent. | OPEN | PAY-04, PAY-08, PAY-09, PAY-10 | none |
| RMT-04 | Two workers, stale leases or a restarted process cannot duplicate a remote attempt or financial effect. | CLOSED_LOCAL | PAY-04, PAY-13 | none |
| RMT-05 | Duplicate refund/event or a reordered event cannot refund or grant twice. | OPEN | PAY-10, PAY-16 | none |
| RMT-06 | When remote absence cannot be proved after timeout, the operation stays uncertain and requires reviewed recovery. | OPEN | PAY-04, PAY-06, PAY-07 | none |

## Webhook authentication and durability

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| WHK-01 | Missing, bad, old or rotated Stripe signature / Asaas `asaas-access-token` is rejected; Asaas API key is not reused as its webhook token. | OPEN | PAY-08, PAY-09 | sandbox |
| WHK-02 | Stripe verification uses the exact bounded raw body; a mutated/re-encoded body is rejected. | OPEN | PAY-08 | sandbox |
| WHK-03 | Wrong merchant/account, live-vs-test mode, API version or resource binding is rejected or quarantined. | OPEN | PAY-08, PAY-09, PAY-10 | sandbox |
| WHK-04 | Huge, malformed or oversized JSON is bounded and rejected without unbounded CPU/memory. | CLOSED_LOCAL | PAY-08, PAY-09, PAY-18 | none |
| WHK-05 | Duplicate event ID with altered content is recorded as a conflict, not reprocessed. | OPEN | PAY-08, PAY-09 | none |
| WHK-06 | Authenticated unknown/additive event causes no grant and is retained minimally for review. | OPEN | PAY-08, PAY-09, PAY-10 | sandbox |
| WHK-07 | DB outage, flood or backlog never returns fake success; Asaas acknowledgement is HTTP 200 only after durable local acceptance. | CLOSED_LOCAL | PAY-08, PAY-09, PAY-18 | none |

## Settlement and access projection

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| STL-01 | Authorized/pending/captured/received states are distinct; only an authoritative settled payment funds access. | OPEN | PAY-03, PAY-10, PAY-11 | sandbox |
| STL-02 | A manually marked-paid Stripe invoice or out-of-band status cannot grant a card entitlement without the approved policy. | OPEN | PAY-10, PAY-11 | sandbox |
| STL-03 | Provider/merchant/customer/order/offer/currency/amount mismatch grants nothing and is surfaced. | OPEN | PAY-10 | sandbox |
| STL-04 | Late, refunded, reversed or under/overpaid settlement follows explicit allocation; no duplicate period. | OPEN | PAY-10, PAY-16 | sandbox |
| STL-05 | Concurrent conventional Pix and recurring/card payment creates at most one access effect; excess money is recorded. | CLOSED_LOCAL | PAY-10, PAY-14, PAY-16 | sandbox |
| STL-06 | A refused Pix Automático authorization can coexist with a successful initial Pix payment without future debit authority. | OPEN | PAY-12 | sandbox |

## Lifecycle and switching

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| LIF-01 | SCA/3DS, action-required, decline and past-due flows remain pending and do not grant unpaid access. | OPEN | PAY-06, PAY-11 | sandbox |
| LIF-02 | Month-end, leap-year, timezone and term anchors produce deterministic paid-through periods. | OPEN | PAY-01, PAY-03 | none |
| LIF-03 | Grace, hold, past-due, cancellation and paid-through expiry preserve only policy-eligible access. | OPEN | PAY-01, PAY-03, PAY-11 | sandbox |
| LIF-04 | Renewal/cancel/refund races cannot resurrect a revoked grant or extend access twice. | OPEN | PAY-11, PAY-14, PAY-16 | sandbox |
| LIF-05 | Expired Pix QR is not payable/reusable and a late payment is reconciled, not blindly granted. | OPEN | PAY-07 | sandbox |
| LIF-06 | Pix Automático refusal, retry, expiry, cancellation and merchant ineligibility stop new instructions. | CLOSED_LOCAL | PAY-12, PAY-13 | sandbox |
| LIF-07 | Switching card/Pix remains single-funding-source; unknown old cancellation blocks a second auto-debit. | CLOSED_LOCAL | PAY-14 | sandbox |
| LIF-08 | An operator/dashboard plan change cannot bypass the offer catalog, proration policy or cross-provider fence. | OPEN | PAY-11, PAY-14 | sandbox |

## Recovery and operations

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| REC-01 | Missed, paused or reordered webhooks are repaired by bounded reconciliation, not per-read provider polling. | OPEN | PAY-08, PAY-09, PAY-10 | sandbox |
| REC-02 | Dead job, provider outage, 429 and bounded retry/backoff preserve unrelated safety work and do not duplicate writes. | CLOSED_LOCAL | PAY-04, PAY-13, PAY-18 | sandbox |
| REC-03 | Deleted/deactivating account stops future billing and cannot auto-refund or keep unnoticed billing. | CLOSED_LOCAL | PAY-15 | sandbox |
| REC-04 | Revoked/rotated provider key is diagnosed safely; maintenance, cancellation and existing access remain operable. | CLOSED_LOCAL | PAY-05, PAY-18 | none |
| REC-05 | Database restore/stale outbox cannot replay a charge/refund or resurrect deleted ownership. | CLOSED_LOCAL | PAY-04, PAY-18 | none |
| REC-06 | Refund with insufficient provider balance or a lost reply stays pending/failed; no blind second refund. | OPEN | PAY-16 | sandbox |
| REC-07 | Chargeback, Pix MED and unmatched account hold affect only matched rights and surface for review. | OPEN | PAY-16 | sandbox |

## Privacy, boundaries and regression

| ID | Required case | State | Owner | Provider proof |
|---|---|---|---|---|
| BND-01 | No PAN/CVV/raw card form, secret or provider credential enters handlers, logs, tests, fixtures or storage. | CLOSED_LOCAL | PAY-02, PAY-06, PAY-18 | none |
| BND-02 | No customer PII, raw QR, payment URL, signature or token appears in logs/artifacts; retention is reviewed. | CLOSED_LOCAL | PAY-05, PAY-18, PAY-20 | none |
| BND-03 | Protected billing retention is separated from account erasure without losing legitimate financial obligations. | CLOSED_LOCAL | PAY-15, PAY-16, PAY-20 | none |
| BND-04 | Payment never changes community trust and cannot buy safety/trust privileges. | CLOSED_LOCAL | PAY-03, PAY-10, PAY-17, PAY-19 | none |
| BND-05 | Disabling sales, expiry or provider outage preserves free data, export and legitimate paid-through access. | CLOSED_LOCAL | PAY-10, PAY-15, PAY-19 | none |
| BND-06 | Enterprise payer/customer identity grants no team, tenant, station or cross-account privilege. | CLOSED_LOCAL | PAY-17 | none |
| BND-07 | No Android map/charging, MAP ingestion, static portal, app purchase UI or unrelated shared-edge regression. | CLOSED_LOCAL | PAY-19, PAY-22, PAY-23 | none |

## Existing PRM reuse candidates

These rows are not payment acceptance. They identify existing source/tests that a
web payment phase may extend after revalidating its own contract. Historical PRM
evidence was not rerun at PAY-00.

| ID | Existing capability | State | Owner | Reuse note |
|---|---|---|---|---|
| PRM-01 | Owner-bound purchase/status/replay and private export. | REUSE_OPEN | PAY-03, PAY-10, PAY-15 | `subscription/application`, `adapters/http`; extend through explicit source-specific ports. |
| PRM-02 | Capability check joins the caller transaction. | REUSE_OPEN | PAY-03, PAY-10 | `subscription/adapters/capability.go`; no separate unchecked cache. |
| PRM-03 | Authenticated provider notification inbox/dedupe and retirement. | REUSE_OPEN | PAY-08, PAY-09, PAY-10 | Schema `000076`/`000077` and provider handler are Play-specific; payment needs separate Stripe/Asaas authentication and inbox identities. |
| PRM-04 | Reconciliation, refresh leases and retired purchase chains. | REUSE_OPEN | PAY-10, PAY-11, PAY-13 | Preserve Play behavior; web sources get their own freshness/lifecycle rules. |
| PRM-05 | Account deletion purges/retires provider material. | REUSE_OPEN | PAY-15 | Migration `000075` trigger; extend without replaying financial writes. |
| PRM-06 | Free status/export remains available when payments are disabled. | REUSE_OPEN | PAY-10, PAY-15 | Preserve `Export` independence from provider configuration. |

## PAY-19 local closure

- AUTH-01: the composed API mounts webhook intake only — HMAC/token-bound
  server callbacks with per-request verification, no owner command, portal
  or purchase surface; forgeries are refused before any store effect.
  See [PAY-19](PAY-19.md).
- AUTH-03: the payment runtime holds no cookie, session-cookie, redirect
  or Play credential surface; callbacks are bearer/token POSTs verified
  per request. See [PAY-19](PAY-19.md).
- BND-04: composed grants carry exactly catalog capabilities through the
  existing subscription projection; the runtime wires no trust, community
  or safety modules. See [PAY-19](PAY-19.md).
- BND-05: flags without credentials enable nothing while owner history/
  status and free data keep working; outage preserves PENDING with paid-
  through access intact. See [PAY-19](PAY-19.md).
- BND-07: payment-inbox runs on its own serial worker slot excluded from
  the general dispatcher; API composition touches no map, charging,
  portal or shared-edge routes; full payments/subscription/api suites
  green. Final protected integration revalidates at PAY-22.

## PAY-18 local closure

- AUTH-05: rotated operator proof fails closed on fresh refund requests,
  completed-intent replays and reconciliation; exactly one provider write
  and one audit row. See [PAY-18](PAY-18.md).
- WHK-04: card/secret-laden huge and malformed bodies are bounded and
  refused without echo or retention. See [PAY-18](PAY-18.md).
- WHK-07: DB outage returns 503 with a static code on both webhooks; a
  64-way duplicate flood converges on one durable inbox row with zero
  grants. See [PAY-18](PAY-18.md).
- REC-02: ambiguous provider outage leaves the refund PENDING with access
  preserved; recovery reconciles the one durable intent exactly once.
  Real 429/backoff windows remain `sandbox` proof at PAY-21.
- REC-04: webhook secret/token rotation accepts the new credential,
  refuses the old-only credential, never reuses the API key as the webhook
  token, and missing configuration fails closed to 503 with maintenance
  intake intact. Live revocation remains `sandbox` proof at PAY-21.
- REC-05: re-driving the settled event after refund is a durable no-op
  (one grant, state preserved); completed refund replay converges without
  a second provider write; stale state transitions are refused as
  conflicts with the terminal record unchanged. See [PAY-18](PAY-18.md).
- BND-01/BND-02: every refusal and intake response is free of PAN, CVV,
  secrets, QR/payment URLs and tokens with `no-store` caching; the inbox
  retains only the payload hash and bounded IDs. See [PAY-18](PAY-18.md).
- Performance: 200-callback intake measured total=174.76ms mean=873.80µs
  p95=1.12ms on disposable PostGIS, inside the frozen ceilings; hot
  inbox/grants/orders/pending queries use indexes with no sequential
  scan. Sandbox timing and production sizing remain PAY-21/PAY-23.

## PAY-17 local closure

- CAT-05: unapproved enterprise offers are refused on card, manual Pix and Pix
  Automático entry points with no order and no provider call; foreign payer,
  entity substitution, quantity/seat overrides and cross-family confusion are
  refused with distinct reasons.
- BND-06: approved enterprise bindings carry zero app capabilities; settled
  enterprise periods grant nothing, premium access stays denied, and no
  team/tenant/station/org schema was introduced.
- Fiscal workflow stays explicitly frozen: no tax engine, fiscal document or
  CNPJ handling was built.

## PAY-16 local closure

- AUTH-04 (refund part, local): duplicate refund intents converge on one local
  row and one provider write; lost-reply reconciliation finds the remote refund
  before any second write. Broader command/webhook dedupe stays with its owners.
- B-BR-PAY-16/17: only verified original-route refunds adjust paid periods;
  pending disputes and unmatched holds never revoke; confirmed losses and
  cumulative full refunds revoke only matched unexpired grants.
- Actual provider fee balances, MED event names and retry windows remain
  PAY-21 sandbox proof; runtime routes/workers remain PAY-19.

## PAY-15 local closure

- BND-03: account deletion keeps `payment_billing_exits` and existing
  order/settlement/grant rows; a real PostGIS test verifies the exit survives the
  deleted account status and no cascade removes financial evidence.
- REC-03: deletion records a durable billing-exit intent before mutation; owner
  cancel/status/history/export and provider-outage/stale-replay paths are tested.
  Actual provider cancellation and worker execution remain PAY-19/PAY-21.

## PAY-14 local closure

- LIF-07: card ↔ Pix Automático and manual-period switches require a confirmed
  old-source fence; unknown cancellation stays pending and blocks activation.
  See [PAY-14](PAY-14.md). Actual provider cancellation/timing remains sandbox.
- STL-05: concurrent manual and automatic settlements produced one grant per
  payment and an explicit `EXCESS` settlement, with no duplicate grant. Provider
  sandbox reconciliation remains PAY-21.

## PAY-13 local closure

- RMT-04: PAY-04 stale-lease/operation fences plus PAY-13 concurrent
  reconciliation of one provider cycle payment, which produced one settlement,
  grant and cycle. See [PAY-13](PAY-13.md).
- LIF-06: refusal/cancellation/expiry and merchant ineligibility stop new
  authority; unknown remote cancellation stays pending and is reconciled. Actual
  provider timing/eligibility remains `sandbox` proof at PAY-21.

## Residual risks and open external proof

- No row is closed by this matrix. PAY-00 created the obligation set only.
- Stripe and Asaas accounts, merchant approval, catalog/webhook registration and
  Pix Automático eligibility are not admitted. API keys alone cannot close rows.
- Exactly-once external writes are not claimed. Asaas `externalReference` is not
  assumed unique; Stripe idempotency keys have limited retention.
- Pix Automático sandbox support is not proven. Unsupported provider simulations
  remain PROOF_OWED at PAY-21, not replaced by mocks or live money.
- The matrix must be updated per phase with command/result/evidence and reviewed at
  PAY-18; PAY-21 adds provider proof and PAY-23 adds the scoped VPS test proof.
