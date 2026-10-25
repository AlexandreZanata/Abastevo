# PAY-02 — provider contracts and SDK admission

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-01
Branch: `codex/payments-stripe-asaas`
Parent: `7bfa2fd4` (PAY-01).

Objective: revalidate the primary provider contracts, pin the Stripe client/API
and select the Asaas client approach after dependency/license/security review;
record exact endpoints, auth, events, status/idempotency limits and sandbox gaps.
No provider account, credential, product, price, webhook, charge or runtime
composition was created.

## Admission result

| Provider | Admitted | Client / license | Account proof |
|---|---|---|---|
| Stripe | API `2026-09-30.endive` | `github.com/stripe/stripe-go/v87` `v87.0.0`, MIT, Go 1.24+ | OWED |
| Asaas conventional Pix | REST v3 | thin `net/http` client; no vendor SDK | OWED |
| Asaas Pix Automático | REST v3, provider-owned `SUBSCRIPTION` | same thin client; `frequency=MONTHLY`, `retryPolicy=NOT_ALLOWED` for v1 | OWED |

Primary sources checked on 2026-10-24:

- Stripe `stripe-go` release `v87.0.0` (2026-10-01), `go.mod`, `api_version.go`
  (`2026-09-30.endive`), `webhook/client.go`, `checkout_session.go`, `invoice.go`,
  `subscription.go`, `charge.go`, `dispute.go`, `event.go`, `params.go` and
  `stripe_client.go`; Stripe webhook, Checkout subscription and idempotency docs.
- Asaas v3 reference for charges, customers, Pix QR, refunds and Pix Automático
  authorization; Asaas retries/idempotency, webhook receipt/FAQ, Pix Automático
  implementation/flows and Sandbox testability pages.

## Frozen contract details

- Stripe: hosted Checkout `mode=subscription`, `allowed_payment_method_types[]=card`,
  server Price ID, stored Customer, owner/order metadata, allowlisted HTTPS
  success/cancel URLs, no promotion/trial/Connect/split/raw card data. The SDK
  `webhook` package verifies the exact raw body and `Stripe-Signature` (300s
  default tolerance); events require API `2026-09-30.endive`, expected
  `livemode`, no unexpected Connect account.
- Stripe settlement: `invoice.paid` is only a candidate. A grant requires
  canonical invoice checks: paid/brl, exact amount, `amount_paid_off_stripe=0`,
  `charge_automatically`, admitted `billing_reason`, matching subscription/
  customer/order metadata, expected mode and a canonical payment reference.
  Checkout completion/redirect alone never grants.
- Stripe writes use stable `Idempotency-Key` values; v1 retention is at least 24
  hours. Timeout remains `OUTCOME_UNKNOWN`; reconcile before retry. SDK default
  network retries (2) are allowed only with the same key/parameters.
- Asaas: base sandbox `https://api-sandbox.asaas.com`, production
  `https://api.asaas.com` (account confirmation at PAY-21); API auth header
  `access_token`; stable `User-Agent`; webhook auth header `asaas-access-token`
  (32–255 chars, not the API key), HTTP 200 only after durable persistence.
- Asaas money is a JSON BRL decimal derived from exact integer centavos; never
  float64. `externalReference` and `contractId` are correlation only, not unique
  idempotency. Reconcile by remote ID/bounded search before retry.
- Conventional Pix: `POST /v3/payments` (`billingType=PIX`, exact value/dueDate/
  customer), QR at `GET /v3/payments/{id}/pixQrCode`; QR payload/encoded image is
  owner-private and never logged. `RECEIVED` is the v1 authoritative settlement;
  `CONFIRMED`, `PENDING`, `OVERDUE` and unknown statuses grant nothing.
- Pix Automático: `POST /v3/pix/automatic/authorizations` with `MONTHLY`,
  `SUBSCRIPTION`, fixed `value`, `immediateQrCode` and bounded correlation;
  `DELETE /v3/pix/automatic/authorizations/{id}` cancels. Initial payment and
  mandate are independent; future debit requires `ACTIVE`; refusal after initial
  receipt keeps only the paid first period. Eligibility (approved PJ, active CNPJ
  ≥6 months, no Pix restriction) remains account-dependent.
- Unknown/additive provider events/statuses are retained minimally and cause no
  financial effect. No status is flattened into a paid boolean and no manual
  crypto/HMAC is invented for Asaas.

## Artifacts

- `docs/planning/STRIPE_ASAAS_PROVIDER_ADMISSION.md` — canonical admission.
- `contracts/testdata/payments/stripe-contracts-v1.json` — synthetic Checkout
  request/response, operations, selected events, invoice verification and refusal
  cases.
- `contracts/testdata/payments/asaas-contracts-v1.json` — synthetic customer/
  payment/QR/authorization/webhook fixtures, operations, 14-status map, 24 events
  and refusal cases.
- Updated provider register status/links, payment plan, `ROADMAP.md` and
  `PROGRESS.md`.
- No `backend/go.mod`/`go.sum` change. The Stripe dependency is selected but will
  be installed at PAY-06 under coordinated `go.mod`/`go.sum` ownership; Asaas
  needs no module.

## Validation performed

```sh
python3 <provider fixture contract assertions>
git diff --check
python3 <added/changed Markdown link check>
bash scripts/scan-secrets.sh
```

Results:

- Provider fixture assertions PASS: Stripe SDK `v87.0.0`, API
  `2026-09-30.endive`, MIT/Go 1.24+, 12 event cases, exact invoice verification
  and 10 refusal cases; Asaas v3 hosts/headers, 14 payment statuses, 24 event
  cases, Pix Automático `SUBSCRIPTION`/`NOT_ALLOWED` and 10 refusal cases.
- No card PAN/CVV, live key, webhook secret, real CPF/CNPJ or scannable Pix
  payload was added; fixtures use clearly synthetic placeholders.
- `git diff --check` PASS; added/changed links PASS; `scan-secrets.sh` PASS.
- No Go build/test is claimed: no Go source or dependency was changed. The
  fixtures are contract assertions, not adapter runtime tests.

## Remaining obligations

- Actual Stripe merchant/account, Price IDs, webhook secret/events/origins and
  test-mode proof at PAY-21.
- Actual Asaas merchant approval, production host/keys, webhook token/events,
  Pix readiness and Pix Automático eligibility/sandbox proof at PAY-21.
- Stripe SDK installation and license/vulnerability check at PAY-06.
- Bounded Pix Automático `finishDate`/duration decision if the mandate is not
  indefinite; any retry-policy change needs a new admission.
- Fiscal/legal/consent and Android distribution gates remain unresolved; all
  channels stay off.

Next action: PAY-03 only after maintainer authorization.
