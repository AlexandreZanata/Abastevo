# PAY-16 — complete refunds, disputes and financial recovery

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-10, PAY-11, PAY-13, PAY-15
Branch: `codex/payments-stripe-asaas`
Parent: `5a1a96e6` (PAY-15).

Objective: refund only the original provider payment route, never create an
automatic or arbitrary payout; reconcile pending/failed/completed refunds,
Stripe chargebacks and Asaas Pix MED/hold lifecycles; expose scoped operator
replay/review behind fresh proof, reason, audit and approval policy.

## B-BR/BUC addressed before implementation

- B-BR-PAY-10/11/13/15: reserve a refund inside the same entitlement guard as
  settlement/cancel and bind every refund to the original settled payment and
  grant; cancellation/expiry never implies a refund.
- B-BR-PAY-16: cumulative pending/succeeded refunds are bounded by the refundable
  original gross under a lock; a failed/requested refund is never complete. Refund
  only to the original provider payment route, never an arbitrary destination.
- B-BR-PAY-17/19/23: a pending dispute is not a fraud finding; a confirmed
  chargeback revokes only the matched unexpired grant; an unmatched hold revokes
  nothing and stays explicit for review; a restored/stale outbox cannot create a
  second remote refund.
- BUC-PAY-01/04/05: owner/account exit never refunds; operator refund execution
  needs fresh proof, bounded amount, reason/audit and approval policy; no manual
  premium credit endpoint is added.

## Implementation

- Migration `000129_payment_refunds_disputes.sql` adds `payment_refunds`
  (operation-key idempotent, provider remote id unique, cumulative principal
  bound), `payment_disputes` (canonical remote id unique), `payment_provider_holds`
  (explicit unmatched money, never access) and `payment_operator_actions`
  (actor/action/target/reason/amount/result audit). It also adds nullable
  `payment_settlements.remote_charge_id` so Stripe disputes/refunds bind the
  original charge/payment-intent to the settled invoice.
- `RefundService` requires fresh operator proof, reserves the refundable balance
  under the entitlement advisory lock before any provider write, refuses amounts
  above the remaining balance (`ErrRefundBounds`), and requires a second
  distinct approver above the configured single-approval limit. Lost provider
  replies stay `PENDING`; only a definitively absent remote refund is retried,
  with the same idempotency key/description, so no blind second write happens.
  `FAILED` frees the reserved balance; terminal rows never move again.
- Completed cumulative full refunds revoke only the matched unexpired grant and
  mark the settlement `REFUNDED`; partial refunds mark `PARTIALLY_REFUNDED` and
  keep access. Settlements without grants (excess) refund without revocation.
- `DisputeService` consumes `charge.dispute.*` and
  `PAYMENT_CHARGEBACK_*`/`PAYMENT_AWAITING_CHARGEBACK_REVERSAL` events, verifies
  them canonically, records disputes with last-event fencing and revokes only on
  confirmed loss against an unexpired matched grant (settlement marked
  `REVERSED`). Duplicate/reordered/out-of-order terminal events are explicit
  no-ops, never re-grants.
- Unmatched money (unknown remote payment/charge on a refund or dispute event)
  is recorded as an `OPEN` provider hold and routed to operator review with
  proof, reason and audit; it never touches grants or access.
- No runtime route/worker composition or provider account was added; no manual
  payout endpoint exists. Runtime wiring and sorting remain PAY-19; only
  synthetic fixtures/sandbox-style answers were used.

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

- Asaas/Stripe adapter tests PASS: refund create/find shape, idempotency and
  provider-error mapping, plus dispute verification and unknown-event ignore.
- PostGIS integration tests PASS:
  - full refund revokes only the matched unexpired grant and marks the
    settlement `REFUNDED`; operator action is audited;
  - partial refund keeps access with `PARTIALLY_REFUNDED`; cumulative full
    refund revokes;
  - concurrent refunds respect the principal (one `SUCCEEDED`, one
    `ErrRefundBounds`) with a single provider write;
  - duplicate operation key replays without a second remote write;
  - lost reply stays `PENDING` and reconciles to `SUCCEEDED` with exactly one
    provider write;
  - insufficient balance fails definitively (`FAILED`), frees the reservation
    and preserves access;
  - above-limit refunds require a distinct second approver; stale/wrong proofs
    are refused;
  - confirmed chargeback loss revokes the matched unexpired grant and access;
    won/duplicate/reversed outcomes never revoke or re-grant;
  - unknown remote money becomes an explicit `OPEN` hold, reviewed with proof
    and audit, without touching grants or access.
- Full backend unit suite PASS. Race payments/subscription suites, vet,
  staticcheck, govulncheck, sqlc, module verification, security static review,
  secret scan, gofmt and diff checks PASS.
- No real Stripe/Asaas account, cancellation, subscription, mandate, charge,
  refund or deployment was used. All sale channels remain disabled.

## Remaining obligations

- PAY-17 completes Enterprise billing and capability-target isolation.
- PAY-18 closes adversarial matrix, privacy/restore and bounded performance
  acceptance.
- PAY-19 wires actual routes/workers/reconciliation for the new refund/dispute/
  hold intents; PAY-21 proves them against real Stripe test mode and Asaas
  sandbox (including exact Pix MED/hold events, fee-affected balances and
  provider retry windows).
- Actual approved operator identities/proofs, single-approval threshold and
  alert routing remain unresolved; the local threshold is a synthetic test knob,
  never a production amount.
- `PROOF_OWED` where the provider can only be proven at runtime: Asaas list-
  refunds shape, Pix MED event names, Stripe charge→invoice correlation edge
  cases.

Next action: PAY-17 only after maintainer authorization.
