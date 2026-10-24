# PAY-06 — Stripe hosted card checkout

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-04, PAY-05
Branch: `codex/payments-stripe-asaas`
Parent: `d519a49b` (PAY-05).

Objective: create/reuse a Stripe customer and hosted Checkout subscription from
authenticated owner, server catalog and durable intent; refuse client-supplied
price/customer/order/return URL/metadata; recover ambiguous creates without
blindly creating another subscription. Real sales remain disabled until account
and sandbox proof.

## Implementation

- `backend/internal/modules/payments/application/` now owns the checkout use
  case and explicit ports. `StartRequest` accepts only operation ID and offer ID;
  any client amount/currency/customer/order/return URL/metadata is refused.
- The order ID is deterministic from `(account, operation)`; the server catalog
  supplies price, BRL amount, accepted capabilities and catalog version. Existing
  orders are reused across retries instead of being re-quoted or recreated.
- The Stripe customer is mapped per owner/provider/environment/merchant and reused.
  Customer creation uses a stable idempotency key and owner metadata; no email or
  card data is accepted from the client.
- Checkout creation claims a durable PAY-04 operation with a stable idempotency
  key, commits `DISPATCHED` before the provider call and persists success/unknown
  outcome. Redirect return grants nothing; no settlement/grant rows are written.
- Recovery: a replay with a persisted remote ID retrieves the same session. After
  an ambiguous/expired lease, the service searches open/all sessions for the bound
  customer and matching `client_reference_id` before any create. A non-definitive
  search returns `ErrAmbiguous` and never blindly creates; a definitive absence
  may create with the same stable key. Recovered completed/expired sessions return
  `ErrStale`; stale workers are fenced by generation.
- `backend/internal/modules/payments/adapters/stripe.go` uses pinned
  `github.com/stripe/stripe-go/v87 v87.0.0` (API `2026-09-30.endive`). Hosted
  Checkout is `mode=subscription`, `allowed_payment_method_types[]=card` (the v87
  filter), server Price ID, exact owner customer, allowlisted config return
  origin, no promotion codes/trial/Connect/raw card data. Metadata is opaque
  order/account/offer/catalog only. Response mode/customer/amount/currency/livemode
  are verified before success.
- Provider errors are mapped to safe domain errors; network/timeout/5xx/429 become
  `ErrAmbiguous`, not a blind retry. `FindCheckout` list failure is never treated
  as proven absence.
- `go.mod`/`go.sum` add Stripe Go `v87.0.0` (MIT; Go 1.24+) and its
  `shopspring/decimal v1.4.0` (MIT). `govulncheck` reports no vulnerabilities.
- Existing PAY-02 fixture/admission was corrected to the v87 field
  `allowed_payment_method_types[]=card`.
- No HTTP route, API/worker wiring, webhook, settlement, grant or sale activation
  is added here; PAY-19 owns runtime wiring.

## Validation performed

```sh
go test -race -tags=integration -count=1 ./internal/modules/payments/...
go test -count=1 ./...
go vet ./internal/modules/payments/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/...
govulncheck ./internal/modules/payments/...
sqlc vet && sqlc diff
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l backend/internal/modules/payments
git diff --check
```

Results:

- Application tests PASS: server-priced checkout; every client tamper field
  refused with no order/provider call; disabled config/catalog/offer refused;
  concurrent starts converge on one session and one provider create; lost
  response → `UNKNOWN` → recovery without a second create; inconclusive search
  stays ambiguous; wrong customer/amount/livemode session refused; completed
  session recovery stale; owner isolation yields account-bound distinct orders.
- Stripe adapter tests PASS with `httptest` (no real account): customer owner
  metadata/idempotency; checkout card-only shape, server Price, exact customer,
  allowlisted URLs, no card/CVC, API version and idempotency headers; session
  mapping; list search found/no-match/error; error mapping; foreign customer
  metadata refused.
- Real PostGIS store integration PASS: happy/replayed checkout, one order/
  operation/attempt/customer, zero settlement/grant rows; lost response recovery
  after lease expiry with one provider create.
- Full backend unit suite PASS. `go vet`, staticcheck, govulncheck, sqlc, security
  static review, secret scan, gofmt and diff checks PASS.
- No real Stripe credential, customer, checkout, subscription, webhook, charge or
  deployment was created by this phase.

## Remaining obligations

- PAY-08/09/10/11 implement webhook intake, settlement verification, grant
  projection and recurring lifecycle; PAY-19 wires authenticated HTTP/runtime.
- A session expiry/completion path currently requires a new checkout operation;
  cancellation/restart UX is part of PAY-11.
- PAY-21 must prove the actual Stripe test account/merchant/Price/webhook journey;
  PAY-23 owns VPS test deployment. All sale switches remain false.
- Error-to-HTTP mapping and rate limits are later runtime work.

Next action: PAY-07 only after maintainer authorization.
