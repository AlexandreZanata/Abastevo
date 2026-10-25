# PAY-10 — verify settlements and project access

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-03, PAY-04, PAY-06–09
Branch: `codex/payments-stripe-asaas`
Parent: `8251aa19` (PAY-09).

Objective: process durable provider events with bounded canonical lookups, commit
the verified settlement/order/grant/cycle/outbox atomically, and let the existing
subscription/capability owner honor web-funded access without Play tokens or Play
credentials. Unknown/invalid/reordered events must not create rights.

## Implementation

- `backend/internal/modules/payments/application/settlement.go` adds a settlement
  processor: it loads the durable inbox event, invokes a provider-specific
  verifier, reloads local order/attempt/customer/catalog, and calls the existing
  PAY-04 `RecordSettlement`. That transaction now writes the settlement, exactly
  one grant, order-paid state, one `payment_cycles` row and the `payment-settlement`
  outbox job. The inbox event is marked `PROCESSED` only after that commit.
- `SettlementService.ProcessPending` re-reads pending inbox rows for reconciliation.
  Provider/DB outage or ambiguous lookup leaves the event `PENDING`; a later retry
  is idempotent. Binding/amount/source mismatches mark `CONFLICT`; ignored/
  non-settled events mark `PROCESSED` with no financial effect.
- Stripe verification (`StripeProvider.VerifySettlement`) retrieves the canonical
  invoice and requires `paid`, `brl`, exact `amount_paid == amount_due`, no
  out-of-band payment, automatic collection, admitted billing reason, matching
  customer/livemode/subscription metadata and a bounded local checkout attempt.
  Manual/send-invoice, pending/open, partial and wrong-currency invoices are
  refused.
- Asaas verification (`AsaasProvider.VerifySettlement`) retrieves the canonical
  payment, accepts only `PAYMENT_RECEIVED`/`RECEIVED`, and binds external reference,
  customer, BRL amount and merchant. `PENDING`/`CONFIRMED`/unknown statuses never
  settle.
- Store additions: `Inbox`, `PendingInbox`, `AttemptForOrder`, `RevokeGrant` and
  the one-cycle insertion inside `RecordSettlement`. Revoking is order-scoped and
  idempotent; a later duplicate settlement event replays the original settlement
  and cannot resurrect a revoked grant.
- Subscription adaptation (coordinated shared path):
  - `RequireCapability` joins the caller transaction and checks active
    `payment_grants` capabilities when Play does not grant the capability.
  - `Status` merges current web grant capabilities/validity into the existing
    snapshot; `Service.Status` reports payments configured when a web grant is
    active even with no Play provider/token codec/binding.
  - Play evaluation, expired/revoked behavior, free data/export and existing
    credential requirements remain unchanged. No fake Play token is created.
- No HTTP route, worker registration, provider secret or sale activation is added;
  runtime wiring remains PAY-19.

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

- Provider verifier tests PASS: canonical Stripe invoice candidate; manual/
  out-of-band, send-invoice, wrong currency, partial, pending and missing metadata
  refused; Asaas `RECEIVED` candidate, pending/ignored events safe.
- Settlement integration tests PASS on real PostGIS:
  - settlement + one cycle + grant + inbox processed; web capability granted
    without any Play purchase/credentials;
  - delayed duplicate and post-revocation replay do not resurrect/duplicate a
    grant; revoked access is denied;
  - provider outage leaves the event pending and `ProcessPending` recovers;
  - wrong amount conflicts without rights; non-settled event is processed without
    rights; two concurrent receipts produce one grant/cycle.
- Subscription integration tests PASS: Play behavior and existing PRM suite
  unchanged; web capability/status work without Play provider; revoking one grant
  preserves an unrelated account's grant and free retained-data access.
- Full backend unit suite PASS. Race integration, vet, staticcheck, govulncheck,
  sqlc, security static review, secret scan, gofmt and diff checks PASS.
- No real provider account, invoice/payment, webhook, credential, sale or
  deployment was used. Tests use synthetic stores/httptest fixtures.

## Remaining obligations

- PAY-11 completes Stripe recurring action-required/grace/cancellation/portal
  lifecycle; PAY-16 completes refunds/disputes/chargebacks beyond the minimal
  revoke primitive and replay fence.
- PAY-14/15/17 cover switching, cancellation/account exits and Enterprise
  projection. PAY-18 owns full backlog/privacy/performance acceptance.
- PAY-19 wires the settlement worker/runtime and subscription reader; PAY-21
  proves actual provider sandbox settlement and webhook delivery.
- All sale switches remain false; no free/local data is gated by payment state.

Next action: PAY-11 only after maintainer authorization.
