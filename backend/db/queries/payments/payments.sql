-- Private payment persistence ports. All writes are transactional; remote
-- network I/O must happen only after the dispatch commit and before outcome
-- persistence. Outbox items reuse job_queue, never a second queue.

-- name: LockPaymentKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(key)::text, 0));

-- name: InsertPaymentCustomer :execrows
INSERT INTO payment_customers(account_id,provider,environment,merchant,remote_customer_id)
VALUES(sqlc.arg(account_id)::uuid,sqlc.arg(provider),sqlc.arg(environment),sqlc.arg(merchant),sqlc.arg(remote_customer_id))
ON CONFLICT DO NOTHING;

-- name: GetPaymentCustomer :one
SELECT * FROM payment_customers
WHERE account_id=sqlc.arg(account_id)::uuid AND provider=sqlc.arg(provider) AND environment=sqlc.arg(environment) AND merchant=sqlc.arg(merchant);

-- name: InsertPaymentOrder :execrows
INSERT INTO payment_orders(
 id,account_id,offer_id,catalog_version,channel,amount_centavos,currency,capabilities,state,quoted_at,expires_at
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(account_id)::uuid,sqlc.arg(offer_id),sqlc.arg(catalog_version),sqlc.arg(channel),sqlc.arg(amount_centavos)::bigint,sqlc.arg(currency),sqlc.arg(capabilities)::text[],sqlc.arg(state),sqlc.arg(quoted_at)::timestamptz,sqlc.arg(expires_at)::timestamptz
)
ON CONFLICT (id) DO NOTHING;

-- name: GetPaymentOrder :one
SELECT * FROM payment_orders WHERE id=sqlc.arg(id)::uuid;

-- name: GetPaymentOrderForUpdate :one
SELECT * FROM payment_orders WHERE id=sqlc.arg(id)::uuid FOR UPDATE;

-- name: MarkPaymentOrderPaid :execrows
UPDATE payment_orders
SET state='PAID',paid_at=sqlc.arg(paid_at)::timestamptz,settlement_id=sqlc.arg(settlement_id)::uuid,
    revision=revision+1,updated_at=clock_timestamp()
WHERE id=sqlc.arg(id)::uuid
  AND (state IN ('PENDING','EXPIRED') OR (state='PAID' AND settlement_id=sqlc.arg(settlement_id)::uuid));

-- name: InsertPaymentOperation :exec
INSERT INTO payment_operations(
 id,account_id,provider,environment,merchant,operation_key,kind,payload_hash,order_id,
 state,generation,attempt_no,lease_until,remote_id
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(account_id)::uuid,sqlc.arg(provider),sqlc.arg(environment),sqlc.arg(merchant),sqlc.arg(operation_key),sqlc.arg(kind),sqlc.arg(payload_hash),
 sqlc.narg(order_id)::uuid,'PENDING',1,1,sqlc.arg(lease_until)::timestamptz,sqlc.narg(remote_id)
);

-- name: GetPaymentOperationByKey :one
SELECT * FROM payment_operations
WHERE account_id=sqlc.arg(account_id)::uuid AND provider=sqlc.arg(provider) AND environment=sqlc.arg(environment)
  AND merchant=sqlc.arg(merchant) AND operation_key=sqlc.arg(operation_key);

-- name: GetPaymentOperationForUpdate :one
SELECT * FROM payment_operations WHERE id=sqlc.arg(id)::uuid FOR UPDATE;

-- name: TakeoverPaymentOperation :execrows
UPDATE payment_operations
SET state='PENDING',generation=sqlc.arg(generation)::bigint,attempt_no=sqlc.arg(attempt_no)::int,
    lease_until=sqlc.arg(lease_until)::timestamptz,updated_at=clock_timestamp(),completed_at=NULL
WHERE id=sqlc.arg(id)::uuid AND generation=sqlc.arg(previous_generation)::bigint
  AND state IN ('PENDING','DISPATCHED','OUTCOME_UNKNOWN')
  AND lease_until IS NOT NULL AND lease_until<=clock_timestamp();

-- name: InsertPaymentAttempt :exec
INSERT INTO payment_attempts(
 id,operation_id,order_id,attempt_no,generation,provider,environment,merchant,payload_hash,
 state,remote_id,lease_until
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(operation_id)::uuid,sqlc.arg(order_id)::uuid,sqlc.arg(attempt_no)::int,sqlc.arg(generation)::bigint,
 sqlc.arg(provider),sqlc.arg(environment),sqlc.arg(merchant),sqlc.arg(payload_hash),sqlc.arg(state),sqlc.narg(remote_id),sqlc.arg(lease_until)::timestamptz
);

-- name: GetPaymentAttempt :one
SELECT * FROM payment_attempts WHERE id=sqlc.arg(id)::uuid;

-- name: GetPaymentAttemptForUpdate :one
SELECT * FROM payment_attempts WHERE id=sqlc.arg(id)::uuid FOR UPDATE;

-- name: GetPaymentAttemptForOrder :one
SELECT * FROM payment_attempts
WHERE order_id=sqlc.arg(order_id)::uuid
ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: MarkPaymentAttemptDispatched :execrows
UPDATE payment_attempts
SET state='DISPATCHED',updated_at=clock_timestamp()
WHERE id=sqlc.arg(id)::uuid AND generation=sqlc.arg(generation)::bigint AND state='CREATED'
  AND lease_until>clock_timestamp();

-- name: MarkPaymentOperationDispatched :execrows
UPDATE payment_operations
SET state='DISPATCHED',updated_at=clock_timestamp()
WHERE id=sqlc.arg(id)::uuid AND generation=sqlc.arg(generation)::bigint AND state='PENDING'
  AND lease_until>clock_timestamp();

-- name: ResolvePaymentAttempt :execrows
UPDATE payment_attempts
SET state=sqlc.arg(state),remote_id=sqlc.narg(remote_id),updated_at=clock_timestamp(),
    completed_at=sqlc.narg(completed_at)::timestamptz
WHERE id=sqlc.arg(id)::uuid AND generation=sqlc.arg(generation)::bigint AND state IN ('DISPATCHED','UNKNOWN');

-- name: ResolvePaymentOperation :execrows
UPDATE payment_operations
SET state=sqlc.arg(state),remote_id=sqlc.narg(remote_id),lease_until=sqlc.arg(lease_until)::timestamptz,
    updated_at=clock_timestamp(),completed_at=sqlc.narg(completed_at)::timestamptz
WHERE id=sqlc.arg(id)::uuid AND generation=sqlc.arg(generation)::bigint
  AND state IN ('PENDING','DISPATCHED','OUTCOME_UNKNOWN');

-- name: InsertPaymentSettlement :execrows
INSERT INTO payment_settlements(
 id,order_id,attempt_id,account_id,offer_id,channel,provider,environment,merchant,
 remote_payment_id,provider_status,authority,amount_centavos,currency,state,outcome,settled_at
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(order_id)::uuid,sqlc.arg(attempt_id)::uuid,sqlc.arg(account_id)::uuid,sqlc.arg(offer_id),sqlc.arg(channel),sqlc.arg(provider),
 sqlc.arg(environment),sqlc.arg(merchant),sqlc.arg(remote_payment_id),sqlc.arg(provider_status),'provider_verified',
 sqlc.arg(amount_centavos)::bigint,'BRL','SETTLED',sqlc.arg(outcome),sqlc.arg(settled_at)::timestamptz
)
ON CONFLICT (id) DO NOTHING;

-- name: GetPaymentSettlementByID :one
SELECT * FROM payment_settlements WHERE id=sqlc.arg(id)::uuid;

-- name: GetPaymentSettlementByOrder :one
SELECT * FROM payment_settlements
WHERE order_id=sqlc.arg(order_id)::uuid AND outcome='GRANTED';

-- name: GetPaymentSettlementByRemote :one
SELECT * FROM payment_settlements
WHERE provider=sqlc.arg(provider) AND environment=sqlc.arg(environment) AND merchant=sqlc.arg(merchant)
  AND remote_payment_id=sqlc.arg(remote_payment_id);

-- name: InsertPaymentGrant :execrows
INSERT INTO payment_grants(
 id,order_id,settlement_id,remote_payment_id,account_id,offer_id,source,paid_centavos,currency,
 start_at,end_at,anchor_day,policy_version,capabilities,state
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(order_id)::uuid,sqlc.arg(settlement_id)::uuid,sqlc.arg(remote_payment_id),
 sqlc.arg(account_id)::uuid,sqlc.arg(offer_id),sqlc.arg(source),
 sqlc.arg(paid_centavos)::bigint,'BRL',sqlc.arg(start_at)::timestamptz,sqlc.arg(end_at)::timestamptz,sqlc.arg(anchor_day)::smallint,
 sqlc.arg(policy_version)::int,sqlc.arg(capabilities)::text[],'ACTIVE'
)
ON CONFLICT (order_id) DO NOTHING;

-- name: GetPaymentGrantByOrder :one
SELECT * FROM payment_grants WHERE order_id=sqlc.arg(order_id)::uuid;

-- name: ListPaymentGrantsForEntitlement :many
SELECT * FROM payment_grants
WHERE account_id=sqlc.arg(account_id)::uuid AND offer_id=sqlc.arg(offer_id)
ORDER BY start_at,id;

-- name: InsertPaymentCycle :execrows
INSERT INTO payment_cycles(id,account_id,offer_id,source,period_start,period_end,state,settlement_id)
VALUES(sqlc.arg(id)::uuid,sqlc.arg(account_id)::uuid,sqlc.arg(offer_id),sqlc.arg(source),sqlc.arg(period_start)::timestamptz,
 sqlc.arg(period_end)::timestamptz,sqlc.arg(state),sqlc.narg(settlement_id)::uuid)
ON CONFLICT (account_id,offer_id,period_start) DO NOTHING;

-- name: GetPaymentCycleByKey :one
SELECT * FROM payment_cycles
WHERE account_id=sqlc.arg(account_id)::uuid AND offer_id=sqlc.arg(offer_id) AND period_start=sqlc.arg(period_start)::timestamptz;

-- name: InsertPaymentInbox :execrows
INSERT INTO payment_inbox(
 provider,environment,merchant,event_id,payload_hash,event_type,resource_type,resource_id
) VALUES(
 sqlc.arg(provider),sqlc.arg(environment),sqlc.arg(merchant),sqlc.arg(event_id),sqlc.arg(payload_hash),sqlc.arg(event_type),sqlc.arg(resource_type),sqlc.arg(resource_id)
)
ON CONFLICT DO NOTHING;

-- name: GetPaymentInbox :one
SELECT * FROM payment_inbox
WHERE provider=sqlc.arg(provider) AND environment=sqlc.arg(environment) AND merchant=sqlc.arg(merchant) AND event_id=sqlc.arg(event_id);

-- name: ListPendingPaymentInbox :many
SELECT * FROM payment_inbox
WHERE provider=sqlc.arg(provider) AND environment=sqlc.arg(environment)
  AND merchant=sqlc.arg(merchant) AND state='PENDING'
ORDER BY received_at,event_id
LIMIT sqlc.arg(limit_count)::int;

-- name: RevokePaymentGrant :execrows
UPDATE payment_grants
SET state='REVOKED',revoked_at=sqlc.arg(revoked_at)::timestamptz
WHERE order_id=sqlc.arg(order_id)::uuid AND state='ACTIVE';

-- name: MarkPaymentInbox :execrows
UPDATE payment_inbox
SET state=sqlc.arg(state),attempts=attempts+1,processed_at=sqlc.arg(processed_at)::timestamptz,last_error=sqlc.arg(last_error)
WHERE provider=sqlc.arg(provider) AND environment=sqlc.arg(environment) AND merchant=sqlc.arg(merchant) AND event_id=sqlc.arg(event_id)
  AND state='PENDING';

-- name: UpsertPaymentContract :one
INSERT INTO payment_contracts(
 id,account_id,offer_id,source,environment,merchant,provider,remote_subscription_id,
 remote_customer_id,provider_status,cancel_at_period_end,last_event_at
) VALUES(
 sqlc.arg(id)::uuid,sqlc.arg(account_id)::uuid,sqlc.arg(offer_id),'stripe_card',
 sqlc.arg(environment),sqlc.arg(merchant),'stripe',sqlc.arg(remote_subscription_id),
 sqlc.arg(remote_customer_id),sqlc.arg(provider_status),sqlc.arg(cancel_at_period_end)::boolean,
 sqlc.arg(last_event_at)::timestamptz
)
ON CONFLICT (provider,environment,merchant,remote_subscription_id) DO UPDATE SET
 account_id=EXCLUDED.account_id,offer_id=EXCLUDED.offer_id,remote_customer_id=EXCLUDED.remote_customer_id,
 provider_status=EXCLUDED.provider_status,cancel_at_period_end=EXCLUDED.cancel_at_period_end,
 last_event_at=EXCLUDED.last_event_at,updated_at=clock_timestamp()
WHERE payment_contracts.last_event_at <= EXCLUDED.last_event_at
RETURNING *;

-- name: GetPaymentContractForAccount :one
SELECT * FROM payment_contracts
WHERE account_id=sqlc.arg(account_id)::uuid AND offer_id=sqlc.arg(offer_id) AND source='stripe_card'
ORDER BY updated_at DESC LIMIT 1;

-- name: GetPaymentContractByRemote :one
SELECT * FROM payment_contracts
WHERE provider='stripe' AND environment=sqlc.arg(environment) AND merchant=sqlc.arg(merchant)
  AND remote_subscription_id=sqlc.arg(remote_subscription_id);
