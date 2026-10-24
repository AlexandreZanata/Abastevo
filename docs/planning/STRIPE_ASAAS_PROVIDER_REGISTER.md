# Stripe and Asaas provider admission register

Status: RESEARCHED / NOT_ACCOUNT_ADMITTED. Date: 2026-10-24.
Owner: [payment plan](STRIPE_ASAAS_PAYMENT_PLAN.md), PAY-01/02/21.

Primary public documentation was inspected in this conversation. No provider
account, credentials, actual webhook, resource schema, eligibility, paid service
or live transaction was configured/validated. Freeze exact SDK/API versions and
actual request/response fixtures at PAY-02; distinguish documented capability
from availability in the maintainer's merchant account. Recheck before activation.

## Stripe card billing

- [Checkout subscriptions](https://docs.stripe.com/payments/checkout/build-subscriptions)
  is the selected hosted-payment direction. The documentation exposes multiple
  UI variants; the hosted variant could not be extracted fully in the planning
  browser. PAY-02 must inspect its exact applicable version and API reference.
  Restrict to admitted card payment methods; no raw card form or Connect/splits.
- [Webhook security](https://docs.stripe.com/webhooks) requires unmodified raw
  request bytes for signature verification. Pin event/API versions and account/
  livemode expectations; test rotation, time tolerance, duplicates and disorder.
- [Subscription events](https://docs.stripe.com/billing/subscriptions/webhooks)
  expose payment failure, action-required and lifecycle changes. Define which
  invoice/settlement facts fund a grant; do not grant merely because checkout
  completed, an invoice exists or an operator marked it paid manually.
- [Idempotent requests](https://docs.stripe.com/api/idempotent_requests) describes
  provider key semantics and pruning after at least 24 hours for API v1. Local
  durable operation/cycle records must outlive remote dedupe. Reusing an old key
  after provider pruning is not proof of safe retry; reconcile uncertain writes.
- [Subscription object](https://docs.stripe.com/api/subscriptions/object) documents
  provider lifecycle and collection modes. Do not flatten incomplete, past-due,
  active, paused collection, canceled and refunded into a single paid Boolean.

Admission record to finish: actual merchant/country/currency/account ownership,
test/live scope, catalog Price IDs and intervals, webhook endpoint/events/version,
API permissions and rotation, SCA/3DS/test cases, checkout/portal return origins,
tax/notices/refund settings and rate-limit/retry behavior. SDK selection requires
license, maintenance/security, Go-version compatibility and pinned dependency
review. No fee, approval or availability claim is made here.

## Asaas conventional Pix

- [Charge reference](https://docs.asaas.com/reference/criar-nova-cobranca) and
  [charge guidance](https://docs.asaas.com/docs/guia-de-cobrancas) define the
  provider resource. PAY-02 freezes supported amount/date/QR/status/expiry fields
  and actual settlement mapping for Pix; do not reuse card-only state semantics.
- [Retries and idempotency](https://docs.asaas.com/docs/retries-and-idempotency)
  directs checking uncertain outcomes before retrying. externalReference is a
  correlation field, not an assumed unique constraint or universal idempotency
  key. Persist and serialize intent, search/reconcile, and quarantine ambiguous
  creation when remote absence cannot be proved safely. Verify each write endpoint.
- [Webhook receipt](https://docs.asaas.com/docs/receive-asaas-events-at-your-webhook-endpoint)
  uses an authToken sent as asaas-access-token; it must differ from the API key.
  Persist before HTTP 200 and process later. This is shared-secret authentication,
  not a per-body asymmetric/HMAC signature.
- [Webhook FAQ](https://docs.asaas.com/docs/webhooks-faq) states at-least-once
  delivery and HTTP 200 success. It describes interrupted delivery after repeated
  failures and a limited retained-event window. Include queue-health detection
  and resource reconciliation rather than relying forever on event redelivery.
- [Charge events](https://docs.asaas.com/docs/webhook-para-cobrancas) describe
  receipts/refunds and other method-specific events. Admit only the selected
  event mappings; preserve unknown additions without financial effects.
- [Pix refund](https://docs.asaas.com/reference/estornar-cobranca) supports
  full/partial refunds with a cumulative principal limit. Fees may leave available
  balance insufficient. Model requested, failed, pending and completed separately.
- [Balance-block events](https://docs.asaas.com/docs/eventos-para-bloqueios-de-saldo)
  can include MED-related holds; some blocks have no associated charge. Such a
  hold cannot identify every affected subscription or prove customer fraud.

Admission record to finish: actual merchant identity/account approval, supported
Pix receipt/QR flow, sandbox/live base origins, API key and User-Agent contract,
minimum required billing data, QR expiration/late-payment behavior, reference
query completeness, webhook token/events and IPs if used as supplemental control,
refund constraints and operator permissions. Do not provision split/subaccounts,
transfers, outgoing Pix or arbitrary beneficiary payments for subscription sales.

## Asaas Pix Automático — separately enabled

The maintainer selected this as well as conventional Pix; neither replaces the
other. [Overview](https://docs.asaas.com/docs/pix-automatico),
[implementation](https://docs.asaas.com/docs/pix-automatico-implementacao),
[authorization reference](https://docs.asaas.com/reference/criar-uma-autorizacao-pix-automatico)
and [webhook flow](https://docs.asaas.com/docs/fluxos-de-webhook) describe payer
authorization, initial payment and later recurring collection.

Select one cycle owner at PAY-01/02. Recommended provider-generated SUBSCRIPTION
mode reduces application scheduling if its actual account/API contract meets
requirements. MANUAL means the application creates authorized debit instructions;
it is not the conventional customer-paid Pix mode. Never run both schedulers.
Provider retries must be admitted with the relevant supported retry policy.

[FAQ](https://docs.asaas.com/docs/faq-2) describes merchant eligibility requirements
and loss of eligibility. Initial payment can succeed even when authorization is
refused. Verify the actual merchant's eligibility before enabling this channel.
[Cancellation reasons](https://docs.asaas.com/docs/motivos-de-cancelamento-da-autoriza%C3%A7%C3%A3o-do-pix-autom%C3%A1tico)
distinguishes canceled authorization from expiry; preserve native reasons and
reconcile already scheduled instructions. Payment receipt alone never activates
future debit authority.

## Sandbox and distribution gates

- [Asaas sandbox FAQ](https://docs.asaas.com/docs/faq-sandbox) and
  [supported tests](https://docs.asaas.com/docs/o-que-pode-ser-testado) describe
  available simulation paths and limitations. No dedicated Pix Automático sandbox
  proof was obtained here; PAY-21 must establish supported cases with the actual
  test account. Unsupported simulations remain PROOF_OWED, not production tests.
- [Google Play payments policy](https://support.google.com/googleplay/android-developer/answer/9858738?hl=pt-BR)
  applies to digital-feature sales in Play-distributed apps and permits alternative
  arrangements only under applicable requirements/programs. Web billing backend
  work may proceed; embedding checkout or purchase steering in Android needs its
  own verified distribution decision. Google provider activation remains LATER.
- [OWASP webhook guidance](https://cheatsheetseries.owasp.org/cheatsheets/Webhook_Security_Cheat_Sheet.html)
  supports authenticated bounded intake, durable acknowledgement and idempotent
  handling. Source-specific provider behavior takes precedence over invented
  common signatures. Follow the project's existing auth/privacy/SQL contracts.

## Required merchant configuration beyond secrets

Record nonsecret merchant IDs, exact test/live environment, product/price mapping,
BRL amounts/terms/capabilities, allowed checkout/portal origins, webhook URLs and
event/version settings, channel switches, Pix readiness/mandate eligibility,
retention/refund/cancellation policies, operational limits and alert routing.
Provider customer notifications must be configured to avoid duplicate/unwanted
email/SMS/WhatsApp and unapproved messaging costs. A billing invoice is not
automatically a Brazilian fiscal document; freeze the required issuance workflow.
No account setup, fiscal decision or real sandbox acceptance is replaced by
inserting an API key.
