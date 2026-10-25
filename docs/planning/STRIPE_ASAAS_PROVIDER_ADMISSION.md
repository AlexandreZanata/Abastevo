# Stripe and Asaas provider admission v1

Status: PROVIDER_CONTRACTS_ADMITTED / ACCOUNT_PROOF_OWED / SALES_DISABLED.
Date: 2026-10-24. Task: PAY-02.
Owner: [payment plan](STRIPE_ASAAS_PAYMENT_PLAN.md).
Sources: [provider register](STRIPE_ASAAS_PROVIDER_REGISTER.md).
Evidence: [PAY-02](../backend/evidence/payments/PAY-02.md).
Fixtures: [Stripe v87](../../contracts/testdata/payments/stripe-contracts-v1.json),
[Asaas v3](../../contracts/testdata/payments/asaas-contracts-v1.json).

This record admits the exact provider contracts, pinned client versions, auth
rules, event/status mappings and retry limitations used by local implementation.
It does not create a provider account, credential, product, price, webhook or
charge. All three sale channels remain disabled and no API key can enable them.

## Admission decision

| Provider | Admitted contract | Client choice | License / toolchain | Status |
|---|---|---|---|---|
| Stripe | API `2026-09-30.endive`; Checkout subscriptions, Billing, Refunds, Disputes | Official `github.com/stripe/stripe-go/v87` `v87.0.0`; use `stripe.NewClient` and the `webhook` package | MIT; requires Go 1.24+ (project uses Go 1.27) | ADMITTED |
| Asaas | REST API `v3` (sandbox and production) | No vendor Go SDK selected; thin `net/http` client with typed bounded responses | Go standard library; no new module | ADMITTED |
| Asaas Pix Automático | REST `v3`, authorization + provider-owned `SUBSCRIPTION` cycles | Same thin client; one cycle owner | Go standard library | ADMITTED; sandbox proof owed |

`backend/go.mod`/`go.sum` are **not** changed by PAY-02. The Stripe SDK is
selected but will be added only when PAY-06 first imports it, after a coordinated
head and the normal dependency/license/vulnerability check. Asaas needs no new
module. This keeps PAY-02 a documentation/fixture checkpoint, not an installed
runtime.

Provider API/SDK versions are pinned. An unknown or newer event/field is retained
minimally and causes no grant; upgrading requires a new admission record and
fixture update.

## Stripe contract

### Operations

| Operation | Method/path | Idempotency | Notes |
|---|---|---|---|
| Create customer | `POST /v1/customers` | `Idempotency-Key` | Store returned ID; bind owner/metadata; never accept client customer ID |
| Create Checkout Session | `POST /v1/checkout/sessions` | `Idempotency-Key` | Hosted card subscription only |
| Retrieve Checkout Session | `GET /v1/checkout/sessions/{id}` | read-only | Reconcile returned session; not payment evidence |
| Expire Checkout Session | `POST /v1/checkout/sessions/{id}/expire` | `Idempotency-Key` | Only for an open stale session |
| Create Billing Portal Session | `POST /v1/billing_portal/sessions` | `Idempotency-Key` | Owner-bound, short-lived, constrained operations |
| Update subscription | `POST /v1/subscriptions/{id}` | `Idempotency-Key` | Approved next-term changes only |
| Cancel subscription | `DELETE /v1/subscriptions/{id}` | `Idempotency-Key` | Stop renewal; paid-through remains |
| Create refund | `POST /v1/refunds` | `Idempotency-Key` | Original payment route, bounded amount |
| Retrieve invoice/subscription/charge/dispute | `GET /v1/invoices/{id}`, `/v1/subscriptions/{id}`, `/v1/charges/{id}`, `/v1/disputes/{id}` | read-only | Canonical verification before any financial effect |

Stripe API v1 idempotency keys are retained for **at least 24 hours**. A stable
operation key is mandatory for each write. Reusing a key after retention is not
proof of safe retry: a timeout is `OUTCOME_UNKNOWN` until reconciled. The SDK
defaults to two network retries; retries are acceptable only with the same stable
key and parameters. No blind create/refund retry after ambiguity.

### Checkout Session request

Use hosted Checkout, `mode=subscription`, cards only:

- `allowed_payment_method_types[]=card` (v87 filter; only card is eligible); no Pix/boleto/Connect/split.
- `line_items[0].price` = server catalog Price ID; `quantity=1`.
- `customer` = stored Stripe customer for the authenticated owner.
- `client_reference_id` = local order ID (not a bearer secret).
- `success_url`/`cancel_url` = allowlisted HTTPS origins; never an arbitrary
  client URL. The success URL grants nothing.
- `metadata` and `subscription_data.metadata` bind order/account/offer; values are
  bounded and contain no PII.
- `allow_promotion_codes=false`; no trial/coupon/installment.
- `locale=pt-BR`; `consent_collection.terms_of_service=required` only after the
  terms URL/text is approved.
- `expires_at` within Stripe's 30-minute to 24-hour window.
- The response `url` is a payment URL: never log, persist in evidence, or return
  to another owner.

### Stripe webhook

- Verify `Stripe-Signature` over the **unmodified raw body** with the maintained
  SDK, not handwritten HMAC. Default tolerance is 300 seconds; configure an
  explicit bounded tolerance.
- Require event `api_version=2026-09-30.endive`, expected `livemode`, and no
  unexpected connected `account` (Connect is out of scope).
- Persist a minimal durable inbox before acknowledging; no provider call in the
  response path. Duplicate event IDs have one effect; altered content for an
  existing ID is a conflict.
- Signature rotation uses overlapping endpoint secrets; keep the old secret only
  for the documented overlap.
- Missing/bad/stale signatures, wrong mode/version and oversized/malformed bodies
  are rejected. A delivery failure never returns success.

Selected events and local effect:

| Event | Local effect | Grant? |
|---|---|---|
| `checkout.session.completed` | Correlate order/session; no settlement | No |
| `checkout.session.async_payment_succeeded` | Correlate; verify canonical invoice/payment | No by itself |
| `checkout.session.async_payment_failed` | Failure state; no grant | No |
| `invoice.paid` | **Settlement candidate**; verify canonical invoice first | Only after verification |
| `invoice.payment_failed` | Payment failure state; no grant | No |
| `invoice.payment_action_required` | SCA/action-required state; no grant | No |
| `customer.subscription.updated` | Lifecycle state; no direct grant/revoke | No |
| `customer.subscription.deleted` | Stop renewal; preserve already-funded period | No |
| `charge.refunded` | Refund candidate; verify original charge/refund | No |
| `charge.dispute.created` | Dispute candidate; no immediate revoke | No |
| `charge.dispute.closed` | Verify outcome; adjust only matched rights | No direct |
| Any unknown/additive event | Retain minimally; no financial effect | No |

`invoice.paid` funds a period only when the canonical invoice satisfies all of:

- `status=paid`;
- `currency=brl`;
- `amount_paid == amount_due == admitted order amount` (integer cents);
- `amount_paid_off_stripe == 0` (manual/out-of-band payment refused);
- `collection_method=charge_automatically`;
- `billing_reason` in `subscription_create`, `subscription_cycle`,
  `subscription_update`;
- `parent.subscription_details.subscription` and customer match the stored
  order/contract; subscription metadata/order binding matches;
- expected `livemode` and merchant environment;
- a canonical payment/charge reference is obtained for refunds/disputes.

A Checkout redirect/completion, an open invoice, an authorized but uncaptured
payment, a manually marked-paid invoice or an unknown state never grants access.
Subscription statuses `active`, `past_due`, `canceled`, `paused`, `unpaid` and
`incomplete` are lifecycle facts, not settlement. Refund statuses `pending`,
`requires_action`, `succeeded`, `failed`, `canceled` and dispute statuses
`needs_response`, `under_review`, `won`, `lost`, `prevented`, `warning_*` are
mapped explicitly; unknown values have no effect.
A source switch stops renewal with
`POST /v1/subscriptions/{id}` and `cancel_at_period_end=true`, then confirms the
canonical subscription before another automatic source may start.

## Asaas v3 contract

### Base, auth and client

- Sandbox: `https://api-sandbox.asaas.com`; production:
  `https://api.asaas.com` (confirm the production host with the admitted account
  before PAY-21).
- API authentication header: `access_token: <API key>`. The key is environment
  specific and never the webhook token.
- Send a stable application `User-Agent` such as `Abastevo/1.0`; do not impersonate
  another client.
- Use HTTPS only, bounded timeouts, response-size limits, no redirects to
  arbitrary hosts, and no provider calls while holding a DB transaction.
- Timeout/connection loss is inconclusive. Reconcile by remote ID or bounded
  search before any retry. `externalReference` is a correlation field, **not** a
  unique key or provider-enforced idempotency.
- Monetary `value` is a JSON decimal in BRL. Convert exact integer centavos to
  decimal text (for example `1234` -> `12.34`); never use `float64` for money.

### Operations

| Operation | Method/path | Correlation | Provider idempotency |
|---|---|---|---|
| Create customer | `POST /v3/customers` | `externalReference` | None; search before retry |
| Find customer | `GET /v3/customers?externalReference=` | `externalReference` | Read-only; may return multiple |
| Create conventional Pix | `POST /v3/payments` with `billingType=PIX` | `externalReference` | None; search before retry |
| Retrieve payment | `GET /v3/payments/{id}` | Provider payment ID | Read-only |
| Find payment | `GET /v3/payments?externalReference=&status=` | `externalReference` | Read-only; not unique |
| Get Pix QR | `GET /v3/payments/{id}/pixQrCode` | Provider payment ID | Read-only |
| Refund payment | `POST /v3/payments/{id}/refund` | Provider payment ID + local refund intent | None; list refunds before retry |
| List refunds | `GET /v3/payments/{id}/refunds` | Provider payment ID | Read-only |
| Create Pix Automático authorization | `POST /v3/pix/automatic/authorizations` | `contractId` (max 35) | None; retrieve/list before retry |
| Retrieve authorization | `GET /v3/pix/automatic/authorizations/{id}` | Provider authorization ID | Read-only |
| List authorizations | `GET /v3/pix/automatic/authorizations?status=&customerId=` | Status/customer | Read-only; `contractId` is not documented as a filter |
| Cancel authorization | `DELETE /v3/pix/automatic/authorizations/{id}` | Provider authorization ID | None; retrieve status before retry |
| Retrieve instruction | `GET /v3/pix/automatic/paymentInstructions/{id}` | Provider instruction ID | Read-only |
| List instructions | `GET /v3/pix/automatic/paymentInstructions?authorizationId=` | Authorization ID | Read-only; bounded census |
| List subscription payments | `GET /v3/payments?subscription=&status=RECEIVED` | Provider subscription ID | Read-only; bounded census |

Required fields are enforced server-side: customer `name` and `cpfCnpj`; Pix
payment `customer`, `billingType`, `value`, `dueDate`; authorization `frequency`,
`contractId`, `startDate`, `customerId` and `immediateQrCode`. For
`SUBSCRIPTION`, `value` is also required. CPF/CNPJ is collected only for the
provider billing flow, minimized and protected; it is not required to create a
free account.

### Conventional Pix and QR

- Create one owner-bound Pix charge per order. Response creation is not payment.
- The QR endpoint returns `encodedImage` (base64), `payload` (copy-paste) and
  `expirationDate`; expose only to the authenticated owner, never log or archive
  the raw payload.
- The server stores the provider payment ID, external reference, exact amount,
  due/expiry and status; it does not share one static QR among orders.
- `dueDate` is required by the provider. Use a server-defined civil date; the
  actual QR expiry comes from `expirationDate`.
- Only `RECEIVED` is the authoritative Pix settlement in v1. `CONFIRMED` means
  paid but funds not yet available and remains pending. `OVERDUE` grants nothing;
  a later `RECEIVED` settles the payment. `RECEIVED_IN_CASH` is not admitted for
  Pix and has no grant.
- Payment status mapping:

| Asaas status | Local class | Grant? |
|---|---|---|
| `PENDING`, `AWAITING_RISK_ANALYSIS`, `CONFIRMED` | pending | No |
| `RECEIVED` | settled | Yes, after canonical verification |
| `OVERDUE` | unpaid/expired | No |
| `REFUND_REQUESTED`, `REFUND_IN_PROGRESS` | refund pending | No new grant |
| `REFUNDED` | full refund | Revoke only matched unexpired period |
| `RECEIVED_IN_CASH` | not admitted | No |
| `CHARGEBACK_REQUESTED`, `CHARGEBACK_DISPUTE`, `AWAITING_CHARGEBACK_REVERSAL` | dispute pending | No immediate revoke |
| `DUNNING_REQUESTED`, `DUNNING_RECEIVED` | ignored | No |
| unknown/additive | unknown | No |

### Pix Automático

Selected mode remains provider-owned `SUBSCRIPTION`, matching PAY-01.

- Authorization: `POST /v3/pix/automatic/authorizations` with `frequency=MONTHLY`,
  `paymentCreationMode=SUBSCRIPTION`, fixed `value`, `contractId`, `startDate`,
  `customerId`, and `immediateQrCode` containing `expirationSeconds` and
  `originalValue`.
- `value` is required for `SUBSCRIPTION`; do not also create application cycles
  in `MANUAL` mode. `MANUAL` would require a new admission and is not a fallback.
- `retryPolicy=NOT_ALLOWED` for v1. Asaas/Pix payer institutions may still retry
  within the same day; extra-day retries are not enabled. Enabling
  `ALLOW_THREE_IN_SEVEN_DAYS` requires a new policy and reconciliation proof.
- `finishDate`/maximum mandate duration, if used, is an owner/config decision.
  Never create an unbounded mandate silently.
- Initial payment and mandate are independent: `PAYMENT_RECEIVED` may arrive
  before `AUTHORIZATION_ACTIVATED`, or the authorization may be `REFUSED` after
  the initial payment. The paid first period remains valid; no future debit is
  allowed unless authorization is `ACTIVE`.
- Authorization statuses: `CREATED`, `ACTIVE`, `CANCELLED`, `REFUSED`, `EXPIRED`.
- Instruction statuses: `AWAITING_REQUEST`, `SCHEDULED`, `REFUSED`, `CANCELLED`.
- Eligibility requires an approved PJ account, active CNPJ, at least six months
  of CNPJ activity and no Pix restriction. Eligibility can change; handle
  `PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED` and stop new instructions when
  ineligible, preserving paid-through.
- Cancellation/expiry stops future instructions; already scheduled instructions
  may be canceled and must be reconciled. The application records the local
  cancellation intent before calling the provider and keeps an unknown outcome
  pending until canonical retrieval confirms it. A source switch uses the same
  path and cannot start another automatic source before confirmation.
- Instruction status `DONE` reports provider processing, not financial
  settlement. Only `PAYMENT_RECEIVED` after canonical verification grants a
  period. No application-side retry instruction is created under `NOT_ALLOWED`.

Selected Pix Automático events:

| Event | Effect |
|---|---|
| `PIX_AUTOMATIC_RECURRING_AUTHORIZATION_CREATED` | Mandate created; no grant |
| `..._ACTIVATED` | Mandate active; future cycles allowed |
| `..._REFUSED` | Mandate refused; initial paid period may remain |
| `..._CANCELLED` / `..._EXPIRED` | Stop future cycles; preserve paid-through |
| `PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_CREATED/SCHEDULED` | Cycle state; no grant |
| `..._REFUSED` / `..._CANCELLED` | Cycle failed/canceled; no grant |
| `PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED` | Channel/eligibility safety |
| `PAYMENT_RECEIVED` | Settlement candidate for that cycle |

### Asaas webhook

- Mandatory `authToken` sent as `asaas-access-token`; 32–255 characters, no
  whitespace, not an API key. This is shared-secret bearer authentication, not a
  per-body HMAC. Optional IP allowlisting is supplemental, never sole trust.
- Persist the event durably by event `id`, then return **HTTP 200**. Duplicate
  deliveries have one effect; altered content for an existing ID is a conflict.
- No long processing or provider call in the callback path. A DB failure returns
  failure, never fake 200.
- Validate `account.id` against the configured merchant. Unknown events are
  retained minimally and cause no effect.
- Asaas retries at least once; after 15 consecutive failures the queue can pause,
  and pending events are retained for up to 14 days. Reconciliation must not rely
  only on redelivery.

## Sandbox and acceptance limits

- Stripe test mode: hosted card/SCA/3DS, invoices, refunds and test clocks are
  expected; actual account configuration and webhook proof remain PAY-21.
- Asaas Sandbox supports customers, Pix charge/QR, refunds and webhooks.
  Chargeback simulation requires a support request. Pix Automático sandbox proof
  was not established in PAY-02; it remains PROOF_OWED until the actual test
  account proves authorization/cycle behavior.
- Missing account, merchant, eligibility, catalog ID, webhook secret or test proof
  keeps the relevant channel disabled. API keys alone are not acceptance.
- No real money, live customer, production webhook or paid service is used in
  this phase.

## Open account-specific items

- Stripe merchant/account ownership, country/BRL support, Price IDs, webhook
  endpoint/secret, allowed origins and tax/consent configuration.
- Asaas merchant/account approval, environment keys, production host confirmation,
  webhook token/events, Pix readiness and Pix Automático eligibility.
- Bounded mandate `finishDate`/duration and retry policy, if changed from v1.
- Exact provider fee/refund balance behavior and notification settings.
- Fiscal/legal review and Android distribution policy remain outside PAY-02.

Next: PAY-03 implements the pure billing/access rules. No provider call or sale
is enabled by this admission.
