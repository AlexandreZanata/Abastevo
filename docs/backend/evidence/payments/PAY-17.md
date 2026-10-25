# PAY-17 — single-payer Enterprise billing and capability-target isolation

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-01, PAY-10, PAY-14, PAY-16
Branch: `codex/payments-stripe-asaas`
Parent: `72725283` (PAY-16).

Objective: bind the exact implemented Enterprise offer to a single accountable
payer and its allowed service/account target; expose owner-safe billing records
and approved provider invoice/receipt references; freeze the fiscal workflow;
prevent company data/email/domain from granting team, station-management or
organization privileges; refuse unimplemented offers.

## B-BR/BUC addressed before implementation

- B-BR-PAY-03/08/15: one capability set per paid period (`sameStrings`), binding
  by order/catalog with server-calculated amounts; a refused or unapproved offer
  never creates an order, a mandate, a charge or access.
- B-BR-PAY-14/16: same-source switching fences stay intact; refunds bind only to
  verified original payments, never to enterprise intent records.
- B-BR-PAY-07/08/15: no provider account, customer, mandate, refund or webhook is
  created by planning or evidence; only synthetic fixtures and disposable local
  databases are used.
- BUC-PAY-01/04/05: owner reads stay private; payment facts never become team,
  station, organization or cross-account privileges; free exits are preserved.

## Implementation

- Domain `DecideEnterpriseBinding` (`backend/internal/modules/payments/domain/contract.go`):
  exactly one approved `enterprise.regional_brief.*` offer, single payer equal
  to the billed account and service target, quantity locked to one, and zero app
  capabilities. Foreign payer/order/target, cross-family offers, seat/quantity
  overrides and capability-bearing enterprise offers are explicitly refused with
  distinct reasons; malformed identity/term/amount still fails closed through
  the frozen offer validation.
- Runtime gate `AuthorizeEnterprisePurchase` on all three sale entries
  (card checkout, manual Pix, Pix Automático): non-enterprise offers pass
  through untouched; enterprise offers require the binding decision, and any
  refusal aborts before order creation, provider calls, mandates or grants.
- `payment_orders` carries `family`; capability-free orders/grants are admitted
  only for the enterprise family (`000130_payment_enterprise_capabilities.sql`,
  append-only). Premium invariants are unchanged: catalogs require premium
  capabilities, binding requires exact capability match, and settlement still
  demands it.
- Billing history exposes the provider invoice/receipt reference
  (`remote_payment_id`) per settled order, scoped to the owning account only.
- Fiscal workflow is frozen by decision, not by code: no tax engine, fiscal
  document, CNPJ handling or receipt synthesis was built; the plan records this
  as an explicit non-goal with provider invoice references as the only money
  trace.
- Shared path `backend/internal/modules/subscription/` was inspected and not
  modified; capability denial reuses the existing projection. No new tenants,
  teams, stations, organizations or cross-account links were introduced anywhere.

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

- Domain tests PASS: approved self-purchase binds; foreign payer, entity
  substitution, zero/multi quantity (seat override), unapproved offer, wrong
  family, premium offer confusion and premium-capability leakage are refused
  with distinct reasons; malformed input fails closed.
- PostGIS integration tests PASS:
  - unapproved enterprise candidate is refused on card checkout, manual Pix and
    Pix Automático with no order and no provider call;
  - approved self-purchase completes checkout and Pix Automático mandate creation
    bound to the exact offer, amount and account;
  - premium payment into an enterprise order is refused as a binding conflict
    with no grant;
  - an approved enterprise settlement records a capability-free paid period,
    grants nothing, and the owner is denied premium capabilities;
  - billing history exposes the provider payment id for the owning account only;
    a foreign account sees nothing.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, module verification, security static review,
  secret scan, gofmt and diff checks PASS.
- No real enterprise deliverable, price, term, provider account, mandate,
  charge or deployment was used. All sale channels remain disabled.

## Remaining obligations

- PAY-18 closes adversarial privacy and performance acceptance.
- PAY-19 wires actual API/worker composition and regressions with MAP/ESP/PRM.
- PAY-21 proves actual provider sandbox journeys (including any future approved
  enterprise offer, price, term, Pix MED holds and fiscal flow).
- Actual approved enterprise price, term, deliverable, fiscal workflow and
  deployment remain unresolved and unbuilt by design.

Next action: PAY-18 only after maintainer authorization.
