# PAY-11 — complete Stripe recurring lifecycle

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-06, PAY-08, PAY-10
Branch: `codex/payments-stripe-asaas`
Parent: `2529e167` (PAY-10).

Objective: model Stripe provider-subscription lifecycle separately from
invoice-funded paid-through grants; process SCA/decline/past-due, renewal,
cancellation and dashboard changes; provide owner-bound hosted management with
approved portal configuration; reconcile missed/stale lifecycle events. No new
entitlement comes from invoice creation.

## Implementation

- Migration `000124_payment_contracts.sql` adds `payment_contracts` with owner,
  offer, card source, test/production environment, merchant, Stripe subscription
  and customer IDs, raw provider status, `cancel_at_period_end`, and
  `last_event_at`. It is separate from `payment_grants`: contracts describe the
  provider subscription; grants describe verified invoice-funded paid-through.
- `StripeProvider` now retrieves canonical subscriptions and maps lifecycle facts
  for `customer.subscription.updated/deleted`, `invoice.payment_failed`,
  `invoice.payment_action_required`, `invoice.paid` and
  `checkout.session.completed`. It requires matching livemode and
  `account_id`/`offer_id` subscription metadata. It creates hosted Billing Portal
  sessions only with an explicit approved `configuration`, owner customer and
  allowlisted return URL.
- `LifecycleService` consumes durable inbox events, upserts the contract with a
  `last_event_at` stale-event fence, marks ignored events processed and leaves
  unknown/provider-outage events pending. `Reconcile` retrieves current provider
  subscription state for missed events. `invoice.created` and provider status
  alone never create a grant.
- `PortalService` requires a stored owner contract, matching environment/merchant,
  approved portal configuration ID and server return origin. It exposes no plan
  change operation and refuses unconstrained configuration. The portal session is
  hosted by Stripe; no card data is handled.
- Cancellation/action-required/past-due only update provider lifecycle state.
  Verified `invoice.paid` grants from PAY-10 remain the sole source of paid-through
  access; a canceled renewal keeps only its already funded period and a stale
  lifecycle event cannot overwrite newer status.
- Store additions: contract upsert/lookup by owner and remote subscription; SQLC
  queries/generated models updated. No runtime/worker/portal route was wired.
- `active`/`past_due`/`canceled` etc. are stored raw; UI mapping and plan-change
  policy remain later runtime work. Grace remains zero under PAY-01, so past-due
  does not extend access.

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

- Stripe adapter tests PASS: subscription lifecycle fact, action-required invoice,
  checkout completion, missing metadata refused, `invoice.created` ignored,
  unconstrained portal refused and constrained portal URL created.
- Lifecycle integration tests PASS on PostGIS:
  - active/trial-style subscription contract stored without grant;
  - SCA/action-required/past-due updates contract but creates no entitlement;
  - cancel/reconcile preserves an already funded grant and stale old status cannot
    overwrite the newer canceled status;
  - `invoice.created` is processed without contract/grant;
  - owner-bound portal uses approved configuration and rejects foreign/missing
    configuration.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, security static review, secret scan, gofmt and
  diff checks PASS.
- No real Stripe subscription, portal configuration, webhook, charge or
  deployment was used.

## Remaining obligations

- PAY-12/13/14 complete Pix Automático and cross-provider switching; PAY-16
  completes refunds/disputes; PAY-18 covers full operational/privacy acceptance.
- PAY-19 wires the lifecycle worker, portal route and subscription runtime; PAY-21
  proves actual Stripe dashboard/portal/SCA journeys. All sales remain false.
- Plan changes/proration are intentionally unsupported until approved catalog and
  portal configuration exist.

Next action: PAY-12 only after maintainer authorization.
