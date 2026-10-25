# PAY-14 — fence plan changes and cross-provider switching

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-11, PAY-13
Branch: `codex/payments-stripe-asaas`
Parent: `5ba72b33` (PAY-13).

Objective: prevent two automatic renewal sources from coexisting for one
account/product family, preserve earned paid-through during switches, make
unknown old-source cancellation block new automatic activation, and reject
unsupported immediate/prorated plan changes.

## B-BR/BUC addressed before implementation

- B-BR-PAY-08/15: one recurring charge owner per target/family. A card or Pix
  Automático source must be fenced with a confirmed provider outcome before the
  other source may activate; unresolved cancellation remains pending.
- B-BR-PAY-09/10: switch intent and target quote are durable and idempotent;
  retries and concurrent devices cannot create a second source or duplicate
  cancellation effects.
- B-BR-PAY-14: the current paid-through window is preserved; a new recurring
  source may debit only at the next boundary. No proration or immediate upgrade.
- B-BR-PAY-15/16: simultaneous manual Pix and automatic payments are recognized
  financially with at most one paid period; excess is recorded, not turned into
  extra access.
- BUC-PAY-05/07: renew/switch with explicit term/consent and stop future billing
  safely.

## Implementation

- Migration `000127_payment_source_switches.sql` adds a durable switch intent:
  target offer/catalog/amount/capabilities, old source/offer/local/remote IDs,
  paid-through snapshot, first-debit boundary, state
  (`PENDING/CANCELING/READY/CONSUMED/CANCELED`), old-cancellation state,
  claim lease/attempts and a unique open switch per account/offer. It is
  append-only after PAY-13.
- Domain `DecidePlanChange` implements the v1 next-term policy. It allows an
  approved same-family monthly target at the current paid-through boundary and
  explicitly refuses proration, immediate/unapproved offers and unsupported
  terms/families. Approved plan/price changes are executed as source
  replacement: the old source is fenced, the target quote is frozen, and the new
  source may start only after the boundary.
- `SwitchService` is idempotent by operation key, rejects a second open switch
  from another device, and fences the old source before reporting `READY`:
  - Stripe card renewal is stopped with `cancel_at_period_end=true` under a
    stable idempotency key and confirmed by canonical subscription retrieval.
  - Pix Automático uses the durable mandate cancellation path, confirms a
    terminal authorization and preserves grants.
  - Provider outage/ambiguous replies leave the switch `PENDING` with
    `old_cancellation=UNKNOWN`; `Reconcile` retries canonical state and never
    reports success early.
- `allowNewRecurringSource` is used by the new-order path of card checkout and
  Pix Automático start. It blocks another source in the same product family,
  requires a `READY` matching switch, rejects a changed catalog/price/capability
  quote (`ErrStale`), and refuses to start before the paid-through/first-debit
  boundary (`ErrBusy`). The old source is never fenced by a client amount or a
  redirect.
- Existing subscription/access ownership was inspected and not changed: the
  switch fence records billing sources only; entitlement grants remain in the
  payments/subscription owners and already preserve paid-through.
- No runtime route/worker or provider account was added. Composition/recurring
  reconciliation remain PAY-19; actual provider switching proof remains PAY-21.
- Shared path `backend/internal/modules/subscription/` was inspected; no file was
  modified. No MAP/ESP path was changed.

## Validation performed

Executed from `backend/` unless noted, using the disposable PostGIS DSN already
used by the payment suite:

```sh
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/...
govulncheck ./internal/modules/payments/... ./internal/modules/subscription/...
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l backend/internal/modules/payments backend/internal/modules/subscription backend/internal/platform/jobs
git diff --check
```

Results:

- Domain tests PASS: next-term approved change uses the paid-through boundary;
  expired boundary applies immediately; proration, same/no-op, disabled offer,
  cross-family and annual-term requests are refused.
- Asaas/Stripe adapter tests PASS, including Stripe
  `cancel_at_period_end=true` with the switch idempotency key and canonical
  lifecycle mapping.
- PostGIS integration tests PASS:
  - card → Pix Automático fences the card, preserves the manual paid period and
    blocks early Pix Automatic activation until the boundary;
  - Pix Automático → card blocks checkout while the mandate is active, cancels
    the mandate, and permits checkout after the confirmed switch;
  - concurrent same-operation requests converge on one switch and one provider
    cancellation; a second device/operation is refused;
  - unknown old cancellation stays `PENDING/UNKNOWN`, blocks the new source and
    reconciles to `READY` after canonical confirmation;
  - a stale catalog/price quote is refused;
  - same-family next-term price changes (cross-source and card-to-card) fence the
    old source and create the target offer only after the boundary;
  - three concurrent settlements (manual + two automatic) produce exactly two
    paid periods and one explicit `EXCESS` settlement, with no duplicate grant.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, module verification, security static review,
  secret scan, gofmt and diff checks PASS.
- No real Stripe/Asaas account, cancellation, subscription, mandate, charge or
  deployment was used. All sale channels remain disabled.

## Remaining obligations

- PAY-15 completes account exit/deletion recovery; PAY-16 completes refunds,
  disputes and excess-money compensation.
- PAY-19 must wire switch routes, periodic reconciliation, cancellation jobs and
  source gates with the real provider clients. The services are local only.
- PAY-21 must prove actual Stripe renewal cancellation/confirmation, Asaas
  mandate cancellation, switch timing and callback recovery. Unsupported cases
  remain `PROOF_OWED`.
- Approved target offers/prices, provider price/plan mappings and fiscal terms
  remain unresolved. No production plan change or sale is authorized.

Next action: PAY-15 only after maintainer authorization.
