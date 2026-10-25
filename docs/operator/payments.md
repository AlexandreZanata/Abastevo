# Payments staging and operations (PAY-20)

Stripe cards, Asaas conventional Pix and Pix Automático run disabled by
default. This runbook covers everything beyond pasting API keys:
catalog, webhook registration, events, API versions, merchant identity,
eligibility, notifications, fiscal boundaries, preflight, rotation,
channel-disable, queue recovery, backup/restore and bounded tests.
Production prices, live modes and any real charge stay separately gated
(PAY-21 commercial acceptance); inserting secrets never activates them.

## Setup beyond API keys

1. Copy `infra/env.staging.example` to the staging env file and fill the
   `REPLACE_ME_*` placeholders with **test-mode** credentials only
   (`sk_test_*`/`rk_test_*`, `whsec_*`, `acct_*`, `price_*` for Stripe;
   sandbox API key, distinct webhook token, merchant UUID for Asaas).
2. Catalog: `ANPFUEL_PAYMENTS_CATALOG_VERSION` stays `pay-catalog-v1`.
   Production prices are unresolved (PAY-01): no offer is sellable and
   checkout refuses with unknown-offer until the commercial catalog lands.
3. Return origin: public `https://` origin of the billing return pages
   (card checkout requires it; loopback `http://` is dev-only).
4. Stripe Dashboard: register the webhook endpoint
   `https://<host>/v1/payments/stripe/webhook` for the pinned API version
   `2026-09-30.endive`, test mode, with invoice, subscription, checkout
   and dispute events; copy the signing secret (`whsec_*`). Restrict the
   API key to the account (`acct_*` merchant must match).
5. Asaas: register `https://<host>/v1/payments/asaas/webhook` with a
   dedicated webhook token (32+ characters, never the API key); the
   handler additionally accepts the previous token during rotation.
   Merchant UUID must match the account. TLS is mandatory at the edge.
6. Portal (optional): create a customer-portal configuration and set
   `ANPFUEL_STRIPE_PORTAL_CONFIGURATION_ID` (`bpc_*`). Empty keeps the
   portal unavailable without affecting anything else.
7. Pix Automático: merchant eligibility is an external Asaas gate; the
   service stays unavailable (`MerchantEligible` false) until the
   authorized eligibility proof lands. Conventional Pix needs no
   eligibility.
8. Notifications: provider email/SMS/WhatsApp and application
   notifications are configured explicitly and never duplicated
   automatically; Asaas customers are created with notifications disabled.
9. Fiscal: provider invoices/receipts are billing references only. No tax
   engine, fiscal document or CNPJ flow exists — fiscal issuance is a
   named owner workflow, not a code path.

## Read-only preflight

Startup logs a redacted preflight (`api.payments`): per channel,
maintenance/new-sales readiness plus blocking reasons
(`disabled-by-flag`, `provider-not-configured`, `mode-mismatch`,
`missing-credentials`, `missing-return-origin`, `missing-catalog`,
`maintenance-disabled`, `invalid-configuration`). Preflight performs no
writes and creates no provider resources. Release criteria:

- Every channel reports a reason (never silently ready).
- Test/staging shows `test` modes only; `live`/`production` modes refuse
  outside production (`mode-mismatch`).
- `NewSalesReady` is false on every channel until sandbox proof.

## Key rotation

- Stripe webhook secret: rotate in the Dashboard, run both secrets
  through the overlap window (the verifier accepts old+new signatures),
  then retire the old one. Old-only signatures are refused.
- Asaas webhook token: set the new token while keeping the previous one
  configured; the handler accepts either during overlap, then clear the
  previous value. The API key is never accepted as the webhook token.
- API keys (`sk_*`/`rk_*`, Asaas key): replace, restart API/worker,
  confirm preflight stays ready and the old key is revoked at the
  provider. Rotation never drops durable inbox rows or paid-through
  access.

## Channel-disable (kill switch)

Set `<channel>_MAINTENANCE_ENABLED=false` and restart. The channel goes
dark: webhook intake answers 503, providers stay unbuilt, inbox rows stay
pending, refunds/switches/portal for that channel refuse. Already-granted
access, free reads and the sibling channel are unaffected. Re-enable by
flipping the flag back; pending rows drain on the next worker tick. Sales
switches (`*_NEW_SALES_ENABLED`) only stop new checkout and never strand
existing subscriptions.

## Queue recovery

- `payment-inbox` jobs are wake-ups: the router drains bounded pending
  rows per configured channel. Missed, reordered or duplicate deliveries
  converge through the same drain — never per-read provider polling.
- Unknown authenticated events are retained minimally with no effect;
  conflicting deliveries are marked `CONFLICT`, never reprocessed.
- Stuck `PENDING` rows: inspect `payment_inbox` (`state`, `attempts`,
  `last_error`, `received_at`); fix the blocking cause (credentials,
  mode, provider outage), then wait for the next tick. Never mark rows
  processed by hand and never delete them: deletion destroys the
  exactly-once guards.
- Dead letters follow the standard job policy (bounded retries with
  backoff, then park for audited replay).

## Backup, restore, rollback

- Financial tables (`payment_*`) ride the standard encrypted backup
  ([backup.md](backup.md)) with the same RPO/RTO; billing exits and
  operator actions are retained across account erasure by policy.
- Restore into an isolated database first: stale outbox/operation rows
  cannot replay a charge or refund (lease, generation and idempotency
  fences), and replayed settlement or refund intents converge without a
  second provider write.
- Rollback is forward-only: no destructive migration exists in the
  payment range. Restoring an older binary over newer rows is refused by
  the migration ledger; roll forward instead.
- After restore, reconcile each channel (drain + settlement/refund
  reconcile paths) before opening the edge.

## Bounded tests (no live money)

- Never load-test provider public endpoints and never simulate sandbox
  cases in production.
- Synthetic journeys run against disposable PostGIS with fixture
  verifiers (see `docs/backend/evidence/payments/PAY-19.md`); provider
  business logic (fees, MED names, retry windows) is proven only with
  real sandbox accounts (PAY-21).
- Callback budget reference (disposable PostGIS, PAY-18): 200 intakes
  total 174.76 ms, mean 873.80 µs, p95 1.12 ms; hot inbox/grants/orders
  queries use indexes with no sequential scan.

## Monitoring signals

Checkout/admission/error rates, provider latency/429/outage, ambiguous
writes, financial/access mismatch census, oldest inbox/job age, dead
letters, webhook rejections/retries, mandate eligibility, pending
refunds, unknown disputes and last reconciliation success. Logs carry
correlation IDs, codes and counts only — never payment URLs, QR data,
tokens, signatures, CPF/CNPJ or customer email.
