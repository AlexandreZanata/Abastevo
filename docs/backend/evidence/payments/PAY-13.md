# PAY-13 — complete Pix Automático recurring cycles

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-12
Branch: `codex/payments-stripe-asaas`
Parent: `e60dcf60` (PAY-12).

Objective: complete provider-owned Pix Automático recurring cycles without
creating application-side debit instructions. Track instruction states, retry
policy/windows, received payments and merchant eligibility; stop future authority
on cancellation, expiry or ineligibility while preserving verified paid periods.

## B-BR/BUC addressed before implementation

- B-BR-PAY-07/08: the selected cycle owner remains Asaas provider-owned
  `SUBSCRIPTION`; the application never creates `MANUAL` charges. Initial payment
  and mandate remain independent.
- B-BR-PAY-09/10: remote instruction and payment IDs are correlated locally;
  duplicate delivery and concurrent workers converge on one settlement, grant and
  cycle. Unknown remote cancellation stays pending.
- B-BR-PAY-13/14: instruction and payment events are reconciled canonically;
  cancellation, expiry and ineligibility stop future authority but do not erase
  earned paid-through or hide a received debit.
- B-BR-PAY-15/18: a second automatic authorization cannot be opened while a
  nonterminal mandate or pending cancellation exists; payment/eligibility facts
  never alter community trust or free access.
- BUC-PAY-04: record consent-backed cycles/retries, reconcile scheduled and
  in-flight instructions, and stop new instructions safely.

## Implementation

- Migration `000126_pix_auto_cycles.sql` adds provider-owned cycle tracking:
  mandate `payment_creation_mode=SUBSCRIPTION`, `retry_policy=NOT_ALLOWED`,
  durable `cancel_requested_at`/`cancel_reason`/`cancelled_at`, inbox
  `resource_status`, `payment_pix_auto_instructions`,
  `payment_pix_auto_eligibility` and a partial unique index allowing only one
  nonterminal mandate per account/offer. It is append-only after PAY-12 and
  changes no applied migration. No MAP/ESP migration number was taken.
- `AsaasProvider` now retrieves/lists Pix Automático payment instructions and
  lists subscription payments. It maps instruction status/purpose/retry/payment
  and authoritative `paymentDate`; automatic settlements are identified by the
  stored subscription ID, never by a client order or external reference.
- `PixAutoCycleService` consumes durable inbox events for authorization,
  instruction and eligibility changes. Instructions are recorded but never
  grant access by themselves; `REFUSED`/`CANCELLED`/`DONE` are retained with a
  stale-event/terminal fence. The cycle owner remains Asaas; no charge, retry or
  application scheduler was added.
- Each verified provider cycle payment receives its own deterministic local
  order/attempt (channel `asaas_pix_automatic`) and goes through the existing
  settlement/grant/cycle transaction. The mandate order remains authorization
  context. Concurrent reconciliation of the same payment converges on one
  order, settlement, grant and cycle; a later cycle gets exactly one new period.
- Cancellation is durable: the local intent is committed before the provider
  call. If the remote outcome is unknown, the mandate stays `ACTIVE` with
  `cancel_requested_at` instead of claiming success; reconciliation retrieves the
  canonical authorization, retries the idempotent cancellation and only then
  records `CANCELLED`/`EXPIRED`/`REFUSED`. A paid period is never revoked by
  cancellation.
- Asaas eligibility is captured from the authenticated webhook payload without a
  provider call in the callback path. `INELIGIBLE` blocks new authorizations
  (even if static admission is enabled), cancels active mandates in bounded
  batches and keeps paid-through access. `ELIGIBLE` clears the local block but
  never reactivates a canceled mandate.
- Reconciliation reads canonical authorization, instructions and received
  subscription payments, settles missing receipts idempotently and retries
  pending cancellations. Incomplete provider listings stay `ErrAmbiguous` rather
  than silently accepting a partial census. A missing local mandate caused by a
  webhook racing the create commit stays pending instead of becoming a
  permanent conflict.
- Shared path inspection: `backend/internal/platform/jobs/` was reviewed. The
  existing durable inbox/outbox already enqueues payment work; no job code,
  worker composition, MAP or ESP file was changed. Runtime wiring/periodic
  scheduling remains PAY-19.

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

- Asaas adapter tests PASS. Pix Automático retrieval reports
  `SUBSCRIPTION`/`NOT_ALLOWED`; instruction retrieval/list preserves status,
  purpose, due date, retry attempt, payment ID and refusal reason; subscription
  payment listing uses the subscription filter; `VerifySettlement` routes a
  subscription payment to `asaas_pix_automatic` with authoritative `paymentDate`.
- PostGIS integration tests PASS:
  - instruction events persist without a grant; concurrent reconciliation of the
    same provider payment produces one settlement/grant/cycle and one cycle order;
  - a second received cycle grants exactly one additional period and order;
  - `REFUSED` instructions grant nothing and a stale `SCHEDULED` update cannot
    regress a terminal instruction;
  - concurrent cancellations with an unknown remote outcome leave the mandate
    `ACTIVE` with durable cancel intent; reconciliation resolves the canonical
    `CANCELLED` state and preserves no false success;
  - `INELIGIBLE` blocks a new authorization even with static eligibility true,
    cancels the active mandate, and preserves an already verified paid period;
  - a second nonterminal automatic authorization is refused without a second
    remote create.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, module verification, security static review,
  secret scan, gofmt and diff checks PASS.
- No real Asaas account, eligibility event, authorization, instruction, Pix
  debit, webhook, sale or deployment was used. All sale switches remain false.

## Remaining obligations

- PAY-14 fences plan/price changes and cross-provider switching; PAY-15 covers
  account exit/deletion; PAY-16 covers refunds/disputes and provider-side
  instruction reversal.
- PAY-19 must route Pix Automático inbox events to this service and run bounded
  mandate reconciliation; no runtime worker or HTTP route is wired here.
- PAY-21 must prove with the actual Asaas test account: eligibility transitions,
  authorization activation/refusal, provider-created instruction lifecycle,
  cancellation timing/in-flight instructions, and the initial payment's
  subscription correlation if the provider omits `subscriptionId`. Unsupported
  sandbox cases remain PROOF_OWED.
- `MerchantEligible` remains false by default; automatic Pix remains
  independently disabled. No production provider proof or live-money acceptance
  is claimed.

Next action: PAY-14 only after maintainer authorization.
