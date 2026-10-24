# PAY-04 — durable financial intents and concurrency guards

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-03
Branch: `codex/payments-stripe-asaas`
Parent: `22feac69` (PAY-03).

Objective: persist payment orders, owner/customer mappings, operation hashes,
remote attempts, settlements, grants, cycles and provider-event inbox state;
fence concurrent remote writes and persist ambiguous outcomes without network I/O
inside transactions. No provider adapter, HTTP route, credential or runtime
composition was added.

## Schema and ownership

- `backend/db/migrations/000123_payment_intents.sql` is append-only. `000122` is
  reserved by the parallel MAP effort; payments uses `000123`. No applied file
  was edited.
- New tables: `payment_customers`, `payment_orders`, `payment_operations`,
  `payment_attempts`, `payment_settlements`, `payment_grants`, `payment_cycles`
  and `payment_inbox`.
- Orders snapshot the capabilities quoted at purchase time; settlement refuses a
  different offer capability set, so a later caller cannot broaden a grant.
- Constraints enforce one owner/remote customer binding, one local operation key
  per owner/provider/environment/merchant, unique remote attempt/payment IDs,
  one granted settlement per order, one grant per order/settlement, one active
  grant start per entitlement, one cycle per period and one inbox event ID.
- `payment_settlements.outcome` distinguishes `GRANTED` from `EXCESS`, so excess
  money is recorded without creating a second grant. `payment_orders` has an
  optional unique settlement identity; `payment_grants` stores its own remote
  payment identity.
- Outbox reuses the existing transactional `job_queue` (`jobs.Enqueue`); no
  duplicate queue was created. Inbox rows contain metadata/hashes only, never raw
  provider payloads, QR payloads, card data or secrets.
- `backend/sqlc.yaml` gained an owned `db/queries/payments` stanza; generated
  pgx code is committed under `backend/db/queries/payments/`.

## Store behavior

`backend/internal/modules/payments/adapters/store.go` provides transactional
primitives:

- `CreateOrder`, `MapCustomer`, `CreateCycle` — idempotent create/converge with
  conflict on changed identity.
- `ClaimAttempt` — advisory-lock + unique operation key, payload hash, live lease
  and generation/attempt-number fence. A live lease returns `ErrBusy`; an expired
  lease takes over with a new generation and attempt number; a terminal operation
  replays; changed payload/kind/order returns `ErrConflict`.
- `MarkDispatched` — commits `PENDING → DISPATCHED` and `CREATED → DISPATCHED`
  before any remote call. Stale generation/attempt rows return `ErrStale`.
- `MarkOutcome` — persists success, failure, `OUTCOME_UNKNOWN`, quarantine or
  cancellation. Unknown keeps a recoverable lease without a terminal completion;
  all terminal outcomes fence on the same generation.
- `RecordSettlement` — locks the order and entitlement, reloads canonical attempt/
  operation/grants, runs the PAY-03 domain policy, then atomically inserts the
  settlement, optional grant, order paid state, resolved attempt/operation and
  outbox job. Webhooks can settle while the attempt is still `DISPATCHED` or
  `UNKNOWN` (webhook-before-reply). Duplicate settlement/remote replay converges;
  concurrent writers produce one grant.
- `RecordInbox`/`MarkInbox` — durable event metadata and idempotent outbox job;
  changed payload hash is a conflict; no provider call occurs in the callback
  transaction.
- No method performs network I/O. The adapter uses bounded transactions and
  returns `ErrUnavailable`, `ErrBinding`, `ErrConflict`, `ErrStale` or `ErrBusy`
  rather than leaking SQL/provider details.

## Validation performed

Integration suite (disposable local PostgreSQL/PostGIS):

```sh
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/adapters
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -tags=integration -count=1 ./internal/platform/migrate
go test -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
go vet ./internal/modules/payments/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/...
sqlc vet && sqlc diff
gofmt -l ...; git diff --check; scripts/scan-secrets.sh
```

Results:

- Ten integration tests PASS, including:
  - order/customer/cycle idempotency and changed-identity conflicts;
  - 12 concurrent claims → one winner, one local operation/attempt; changed
    payload conflict; expired-lease takeover generation 2; stale dispatch refused;
  - webhook-before-reply settlement and exact replay with one settlement/grant/job;
  - ambiguous outcome persisted as `UNKNOWN`/`OUTCOME_UNKNOWN`, recovery after
    lease expiry, stale outcome refused;
  - inbox/outbox duplicate/created/changed-payload behavior;
  - forced grant-insert failure rolls back settlement, order, attempt and outbox;
  - 8 concurrent identical settlements → one granted effect and one job;
  - same-order excess and capacity excess recorded as `EXCESS` without a second
    grant;
  - migration upgrade from pre-123, replay no-op, PostGIS available, all payment
    tables present.
- `go test -race -tags=integration ./internal/modules/payments/adapters` PASS.
- Existing subscription unit/integration suites PASS; PAY-03 domain tests PASS.
- `go vet`, staticcheck, `sqlc vet`, `sqlc diff`, `gofmt`, diff check and secret
  scan PASS.
- No provider account, HTTP handler, runtime wiring, deployment, live sale or
  real customer/payment data was created.

## Remaining obligations

- PAY-05 must validate configuration/credential presence and independent channel
  activation; missing values remain disabled.
- PAY-06/07/08/09/10/11/12/13/14/16/19 still need provider adapters, event
  signature/authorization handling, projection into the existing subscription
  owner, lifecycle, reconciliation and runtime wiring.
- PAY-21 must prove the actual sandbox/account journeys. Threat-matrix rows remain
  OPEN until adapter/integration/provider proof exists.
- No production price, Enterprise offer, fiscal/legal consent or live money is
  enabled by this phase.

Next action: PAY-05 only after maintainer authorization.
