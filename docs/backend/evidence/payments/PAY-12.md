# PAY-12 — Pix Automático consent and authorization

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-07, PAY-09, PAY-10
Branch: `codex/payments-stripe-asaas`
Parent: `65ffd486` (PAY-11).

Objective: create an admitted Asaas Pix Automático authorization with explicit
payer consent and merchant eligibility, track the mandate independently from the
initial paid period, and support retrieval/refusal/expiry/cancellation/audit
without assuming activation. Automatic Pix remains independently disabled until
real provider proof.

## Implementation

- Migration `000125_payment_mandates.sql` adds `payment_mandates`: owner/offer,
  initial order, test/production, Asaas merchant/customer, remote authorization
  and subscription IDs, 32-character `contractId`, status
  (`CREATED/ACTIVE/REFUSED/CANCELLED/EXPIRED`), frequency/value/start/finish,
  consent version/time and `last_event_at`. No CPF/CNPJ, QR payload or provider
  secret is stored there.
- `AsaasProvider` now creates/retrieves/lists/cancels Pix Automático
  authorizations. Creation is `MONTHLY`, `SUBSCRIPTION`, fixed server value,
  `retryPolicy=NOT_ALLOWED`, bounded immediate QR, `contractId` correlation and
  `finishDate` from a conservative 12-month default. Responses are bounded and
  status/value/customer/contract are validated. No auto-debit is assumed from
  authorization creation.
- `PixAutoService.Start` requires explicit `Consent=true`, payer data only for
  first customer creation, a server catalog quote, merchant-eligibility flag,
  consent version, durable order/attempt and the selected provider-owned cycle
  mode. Lost replies and expired leases use `FindPixAutoAuthorization` before
  create; inconclusive search remains `ErrAmbiguous` and never blindly authorizes.
- Initial paid period and mandate are independent: the authorization/order is
  stored with `CREATED/REFUSED` etc., while a canonical `PAYMENT_RECEIVED`
  settlement can still fund the first period via the stored remote subscription
  ID. A refused/expired/cancelled mandate exposes no future-debit authority but
  does not erase the funded first period.
- `Status` is owner-bound, retrieves the canonical authorization and persists
  refusal/activation/expiry. `Cancel` is owner-bound and stops future authority;
  it does not revoke an already funded period. Foreign owners and wrong customers
  are denied. Consent version/time are privacy-safe audit fields.
- The service requires `NewSalesEnabled` and `MerchantEligible`; the latter is an
  external account/proof gate and remains false by default at runtime. No worker,
  route, webhook processing, mandate renewal or live authorization is added.

## Validation performed

```sh
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./internal/modules/subscription/...
govulncheck ./internal/modules/payments/... ./internal/modules/subscription/...
sqlc vet && sqlc diff
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l backend/internal/modules/payments backend/internal/modules/subscription
git diff --check
```

Results:

- Asaas adapter tests PASS: authorization shape (frequency/contract/customer/
  SUBSCRIPTION/NOT_ALLOWED/immediate QR), retrieval/refusal, list by customer and
  contract, cancellation.
- Pix Automático integration tests PASS on PostGIS:
  - explicit consent and merchant eligibility required; consent audit persisted;
  - initial `PAYMENT_RECEIVED` with a `REFUSED` mandate funds exactly the first
    paid period and still exposes no future-debit authority;
  - cancellation/expiry updates mandate state while preserving funded access;
  - lost authorization reply recovers from durable search without a second create;
    inconclusive search stays pending/ambiguous;
  - foreign status/cancel, wrong customer and missing consent are denied.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, security static review, secret scan, gofmt and
  diff checks PASS.
- No real Asaas account, eligibility, authorization, Pix debit, webhook, sale or
  deployment was used. All sale switches remain false.

## Remaining obligations

- PAY-13 implements provider-owned recurring cycle/instruction handling, retry
  windows, eligibility changes and cancellation races. PAY-14/16 cover switching
  and refunds/disputes.
- PAY-19 wires authenticated start/status/cancel routes and lifecycle workers;
  PAY-21 must prove actual Asaas eligibility/authorization/first-payment/refusal.
- `MerchantEligible` stays false until that proof; automatic Pix remains
  independently disabled even if conventional Pix/card are later enabled.

Next action: PAY-13 only after maintainer authorization.
