# PAY-15 — complete billing cancellation and account exits

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-14
Branch: `codex/payments-stripe-asaas`
Parent: `a9b2903f` (PAY-14).

Objective: make owner cancellation and account deletion durable and visible,
separate stopping renewal from expiry/payment/refund, preserve financial records
and free exits, and prevent deletion from creating refunds or silently leaving
automatic billing.

## B-BR/BUC addressed before implementation

- B-BR-PAY-14/18: cancellation stops future renewal only; verified paid-through
  and free data/export remain available. Disabling sales/maintenance does not
  erase settled rights.
- B-BR-PAY-19/20: deletion records a durable billing-exit intent before the
  account mutation, keeps only minimal financial metadata and never triggers a
  refund or a new charge.
- B-BR-PAY-22/23: provider operational failure leaves cancellation visible as
  pending; no fake success and no provider call in an unauthenticated path.
- BUC-PAY-07: owner/account exit stops future billing safely and preserves the
  approved free exits.

## Implementation

- Migration `000128_payment_billing_exits.sql` adds account-level exit intents
  and per-source cancellation rows. Sources snapshot the provider, environment,
  merchant, offer, local ID and remote ID; states are
  `PENDING/CANCELING/DONE/REVIEW` with an audited lease and bounded attempts.
  No refund table or manual grant path is added.
- `BillingExitService` provides:
  - owner-authenticated `Cancel` with fresh owner proof and operation idempotency;
  - owner-bound `Status`, `History` and proof-protected `Export` (billing
    records only; no provider call on read);
  - internal `RequestAccountExit` for account deletion, which records the intent
    and never refuses deletion after that durable write;
  - `Reconcile` for provider outages/stale outbox replay.
- Card cancellation uses the PAY-14 idempotent `cancel_at_period_end` fence and
  confirms canonical state. Pix Automático retrieves the authorization, confirms
  customer/contract binding and only records `DONE` after a terminal status. A
  provider outage leaves the source `PENDING` with a bounded error; an
  unprovable/binding state goes to `REVIEW`.
- `allowNewRecurringSource`/PAY-14 gates remain the only source-creation path.
  Cancellation does not revoke grants, expire paid-through, issue a refund or
  delete local data/export.
- Account integration: `account.Service.DeleteAccount` now requires a
  `RequestBillingExit` port and calls it before the account row mutation. A
  missing/failing intent refuses deletion before mutation. The API composition
  records the durable intent (provider execution remains PAY-19); account tests
  prove billing is attempted before privacy erasure.
- Minimal retention: account deletion does not cascade to payment orders,
  settlements, grants, exits or history. The real PostGIS test keeps the billing
  exit after the account status becomes deleted.
- Shared paths: `backend/internal/modules/account/` was changed additively for
  the explicit exit hook; `backend/internal/modules/privacy/` was inspected and
  not modified. No MAP/ESP path changed. Runtime cancellation worker/HTTP routes
  remain PAY-19.

## Validation performed

Executed from `backend/` unless noted, using the disposable PostGIS DSN already
used by the payment suite:

```sh
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/modules/account/...
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/... \
  ./internal/modules/account/... ./internal/platform/jobs/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... \
  ./internal/modules/subscription/... ./internal/modules/account/... ./internal/platform/jobs/...
govulncheck ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/modules/account/...
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l backend/internal/modules/payments backend/internal/modules/subscription \
  backend/internal/modules/account backend/internal/platform/jobs backend/cmd/api
git diff --check
```

Results:

- PostGIS/account tests PASS:
  - card and Pix Automático owner exits record sources and cancel only the
    matched renewal; a provider outage leaves `PENDING` with visible error;
  - a fenced card is reconciled from stale state without a second remote cancel;
  - account deletion records a durable intent even when provider clients are
    absent, and the source rows remain `PENDING` rather than silently succeeding;
  - account deletion refuses before mutation when billing wiring is missing or
    the billing intent fails; successful deletion records billing before erasure;
  - foreign/revoked/other-account proof is refused and creates no exit;
  - status/history/export are owner-bound; export requires fresh proof;
  - account deletion retains the billing exit row and minimal financial records;
  - no refund path is reachable from cancellation or account exit.
- Full backend unit suite PASS. Race payments/subscription/account suites, vet,
  staticcheck, govulncheck, sqlc, module verification, security static review,
  secret scan, gofmt and diff checks PASS.
- No real provider cancellation, refund, charge or deployment was used. All sale
  channels remain disabled.

## Remaining obligations

- PAY-16 completes refunds, disputes/MED and restricted operator recovery.
- PAY-19 wires the actual exit HTTP routes, provider clients and periodic
  reconciliation worker. In the current local composition, account deletion
  records the source rows but leaves provider cancellation `PENDING` until that
  worker/providers are wired; this is explicit, not a false success.
- PAY-21 must prove actual provider cancellation/expiry and callback recovery.
- Account deletion is intentionally fail-closed when the billing exit hook is
  absent; production activation requires the PAY-19 composition.

Next action: PAY-16 only after maintainer authorization.
