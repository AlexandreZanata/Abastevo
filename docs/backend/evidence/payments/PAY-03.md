# PAY-03 — deterministic billing and access policy

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-01, PAY-02
Branch: `codex/payments-stripe-asaas`
Parent: `efe5f170` (PAY-02).

Objective: implement provider-neutral, deterministic order/attempt/settlement/
contract/mandate/refund/dispute/grant rules with exact money, calendar periods,
source allocation and access projection. Preserve existing Play and free-exit
behavior. No database, HTTP, provider, job or runtime composition was added.

## Implementation

New pure package: `backend/internal/modules/payments/domain/`.

- `policy.go`: policy version, exact `Money` in BRL centavos, channel/provider/
  environment/term enums, versioned offer catalog, server-only quotes, quote TTL,
  offer/capability/price/channel validation, Premium capability bundle.
- `order.go`: separate `Order` and remote `Attempt` state machines. Quotes cannot
  become paid without an order/attempt; successful attempts alone still do not
  grant; ambiguous outcomes stay `UNKNOWN` until success/failure/quarantine.
- `settlement.go`: authoritative settlement binding (merchant, environment,
  provider, channel, attempt, order, account, offer, amount, currency), verifier
  authority, paid periods, grants, access, refunds and disputes.
- `contract.go`: recurring contract/mandate transitions, one recurring funding
  source per account/offer, and deterministic source-switch decisions.
- `golden_test.go` / `policy_test.go`: golden fixture, all state permutations,
  invalid transitions and adversarial authority cases.

Key frozen behavior:

- Only `AuthorityProviderVerified` + `SETTLED` + a succeeded attempt can fund a
  period. Redirect, authorization, manual/out-of-band invoice and unknown provider
  state are refused. Checkout completion alone never grants.
- One order/payment funds one immutable period. Early renewal starts at current
  paid-through; late renewal starts at settlement and resets the anchor; month-end
  and leap boundaries clamp; grace is zero. At most one future period is allowed;
  extra money becomes explicit `Excess`, never extra access.
- A grant exposes exactly the existing seven Premium capabilities and keeps
  `RetainedDataAccess=true` in every state, including expired/revoked/no purchase.
- Partial refunds are financial-only. Cumulative full refund or confirmed lost
  dispute revokes only the matched grant's unexpired period. Pending/under-review
  disputes do not revoke. Refund reservations are bounded and released on failure.
- One recurring source per account/offer. Switching takes effect at the next term
  boundary with no proration; an unresolved old cancellation blocks the new
  automatic source.
- Unknown provider events/statuses are not mapped into a paid boolean; the domain
  accepts only explicit settled facts and refuses everything else.

Existing `backend/internal/modules/subscription/domain/` was not modified. A
regression test imports it and verifies the same seven capabilities and free
retained-data access. The full existing subscription unit suite also passes.

## Validation performed

Commands run from `backend/`:

```sh
gofmt -w internal/modules/payments/domain/*.go
go test -race -count=1 ./internal/modules/payments/domain ./internal/modules/subscription/domain
go test -count=1 ./internal/modules/subscription/...
go vet ./internal/modules/payments/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/...
go test -cover ./internal/modules/payments/domain
```

Results:

- Payment domain and subscription-domain race tests PASS.
- All existing subscription unit packages PASS (`adapters`, `googlepush`, `http`,
  `jobs`, `play`, `application`, `domain`).
- `go vet` PASS; staticcheck PASS with the repository's pinned Go 1.27.1 toolchain.
- Coverage: 73.8% of statements in the new domain package (including every
  admitted state permutation and all 26 PAY-01 golden cases).
- Golden fixture checks PASS for period/time boundaries, access, refunds,
  disputes, excess, amount mismatch, switching and disabled sale paths.
- No SQL migration, DB test, HTTP handler, provider call, credential, deployment,
  PR or sale was created. Threat-matrix rows remain OPEN until persistence,
  adapters, integration and provider proof.
- `gofmt` clean, `git diff --check` PASS, added/changed Markdown links PASS and
  `scripts/scan-secrets.sh` PASS.

## Remaining obligations

- PAY-04 must persist intents, settlements, grants, refunds/disputes and
  idempotency/concurrency guards on real PostgreSQL/PostGIS.
- PAY-06/07/08/09/10/11/12/13/14/16/19 must implement adapters, event intake,
  projection into the existing subscription owner, lifecycle, reconciliation and
  runtime wiring. Pure policy tests are not provider or deployment acceptance.
- Production prices, Enterprise deliverable/rights, tax/fiscal/legal consent and
  all provider accounts remain unresolved/disabled.
- Existing Play sandbox proof remains owed separately; no Play behavior was
  reaccepted or changed here.

Next action: PAY-04 only after maintainer authorization.
