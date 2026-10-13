-- Ownership joins are reviewed module ports: never import account/identity adapters.
-- name: LockOwner :one
SELECT id FROM accounts WHERE id=sqlc.arg(account_id)::uuid AND status='active' FOR UPDATE;

-- name: LockSession :one
SELECT id FROM account_session_families
WHERE id=sqlc.arg(family_id)::uuid AND account_id=sqlc.arg(account_id)::uuid
AND revoked_at IS NULL AND access_expires > clock_timestamp() FOR SHARE;

-- name: LockBinding :one
SELECT contributor_id FROM account_contributor_bindings
WHERE account_id=sqlc.arg(account_id)::uuid AND contributor_id=sqlc.arg(contributor_id)::text FOR SHARE;

-- name: LockSigningKey :one
SELECT k.id FROM identity_keys k JOIN identity_contributors c ON c.id=k.contributor_id
WHERE k.id=sqlc.arg(key_id)::uuid AND c.id=sqlc.arg(contributor_id)::uuid
AND k.revoked_at IS NULL AND c.status='active' FOR SHARE OF k,c;

-- name: GetOperation :one
SELECT * FROM subscription_operations
WHERE account_id=sqlc.arg(account_id)::uuid AND operation_id=sqlc.arg(operation_id)::uuid FOR UPDATE;

-- name: HourlyAdmissions :one
SELECT count(*) FROM subscription_operations WHERE account_id=$1 AND created_at > clock_timestamp()-interval '1 hour';

-- name: PendingAdmissions :one
SELECT count(*) FROM subscription_operations WHERE account_id=$1 AND status='pending' AND lease_until>clock_timestamp();

-- name: ClaimSlot :one
INSERT INTO subscription_verification_slots(token_digest,generation,account_id,operation_id,lease_until)
VALUES(sqlc.arg(token_digest),1,sqlc.arg(account_id),sqlc.arg(operation_id),clock_timestamp()+interval '20 seconds')
ON CONFLICT (token_digest) DO UPDATE SET generation=subscription_verification_slots.generation+1,
account_id=EXCLUDED.account_id,operation_id=EXCLUDED.operation_id,lease_until=EXCLUDED.lease_until
WHERE subscription_verification_slots.lease_until<=clock_timestamp()
RETURNING generation;

-- name: GetSlot :one
SELECT *, (lease_until>clock_timestamp())::boolean AS lease_live FROM subscription_verification_slots WHERE token_digest=$1 FOR UPDATE;

-- name: StartOperation :exec
INSERT INTO subscription_operations(account_id,operation_id,kind,token_digest,payload_hash,token_cipher,generation,lease_until)
VALUES(sqlc.arg(account_id),sqlc.arg(operation_id),sqlc.arg(kind),sqlc.arg(token_digest),sqlc.arg(payload_hash),sqlc.arg(token_cipher),sqlc.arg(generation),clock_timestamp()+interval '20 seconds')
ON CONFLICT (account_id,operation_id) DO UPDATE SET generation=EXCLUDED.generation,token_cipher=EXCLUDED.token_cipher,
attempts=subscription_operations.attempts+1,lease_until=EXCLUDED.lease_until;

-- name: FinishOperation :execrows
UPDATE subscription_operations SET status=sqlc.arg(status),completed_at=clock_timestamp(),token_cipher=NULL
WHERE account_id=sqlc.arg(account_id)::uuid AND operation_id=sqlc.arg(operation_id)::uuid
AND generation=sqlc.arg(generation)::bigint AND status='pending';

-- name: ReleaseSlot :exec
UPDATE subscription_verification_slots SET lease_until=clock_timestamp()
WHERE token_digest=sqlc.arg(token_digest) AND generation=sqlc.arg(generation);

-- name: GetPurchase :one
SELECT * FROM subscription_purchases WHERE token_digest=$1;

-- name: GetRetiredToken :one
SELECT account_id FROM subscription_retired_tokens WHERE token_digest=$1;

-- name: RetireToken :one
INSERT INTO subscription_retired_tokens(token_digest,account_id) VALUES(sqlc.arg(token_digest),sqlc.arg(account_id))
ON CONFLICT (token_digest) DO UPDATE SET account_id=subscription_retired_tokens.account_id
WHERE subscription_retired_tokens.account_id=EXCLUDED.account_id
RETURNING account_id;

-- name: RetirePurchase :exec
UPDATE subscription_purchases SET retired=true,revision=revision+1,token_cipher=NULL
WHERE token_digest=sqlc.arg(token_digest) AND account_id=sqlc.arg(account_id);

-- name: SavePurchase :one
INSERT INTO subscription_purchases(token_digest,account_id,environment,package_name,product_id,base_plan_id,state,valid_until,verified_at,token_cipher,acknowledged,linked_digest)
VALUES(sqlc.arg(token_digest),sqlc.arg(account_id),sqlc.arg(environment),sqlc.arg(package_name),sqlc.arg(product_id),sqlc.arg(base_plan_id),sqlc.arg(state),sqlc.narg(valid_until),sqlc.arg(verified_at),sqlc.arg(token_cipher),sqlc.arg(acknowledged),sqlc.narg(linked_digest))
ON CONFLICT (token_digest) DO UPDATE SET state=EXCLUDED.state,valid_until=EXCLUDED.valid_until,
verified_at=EXCLUDED.verified_at,revision=subscription_purchases.revision+1,refresh_required=false,
token_cipher=EXCLUDED.token_cipher,acknowledged=EXCLUDED.acknowledged,linked_digest=EXCLUDED.linked_digest,
product_id=EXCLUDED.product_id,base_plan_id=EXCLUDED.base_plan_id
WHERE subscription_purchases.account_id=EXCLUDED.account_id AND NOT subscription_purchases.retired AND subscription_purchases.state<>'REVOKED'
RETURNING token_digest;

-- name: ReadStatus :one
SELECT * FROM subscription_purchases WHERE account_id=$1
ORDER BY (NOT retired AND NOT refresh_required AND state IN ('ACTIVE','CANCELED','GRACE')
AND valid_until>clock_timestamp() AND verified_at>clock_timestamp()-interval '15 minutes') DESC,
verified_at DESC,token_digest LIMIT 1;

-- name: LockToken :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(token_digest)::text,0));

-- name: GetProviderReceipt :one
SELECT payload_hash FROM subscription_provider_notifications WHERE environment=sqlc.arg(environment) AND message_id=sqlc.arg(message_id);

-- name: NotificationAdmissions :one
SELECT count(*) FROM subscription_provider_notifications WHERE received_at>clock_timestamp()-interval '1 minute';

-- name: InsertProviderReceipt :exec
INSERT INTO subscription_provider_notifications(environment,message_id,payload_hash,token_digest)
VALUES(sqlc.arg(environment),sqlc.arg(message_id),sqlc.arg(payload_hash),sqlc.arg(token_digest));

-- name: InvalidatePurchase :exec
UPDATE subscription_purchases SET refresh_required=true,revision=revision+1 WHERE token_digest=$1 AND NOT retired;

-- name: InvalidateSlot :exec
UPDATE subscription_verification_slots SET generation=generation+1,lease_until=clock_timestamp() WHERE token_digest=$1;

-- name: MarkAcknowledged :execrows
UPDATE subscription_purchases SET acknowledged=true WHERE token_digest=sqlc.arg(token_digest)
AND revision=sqlc.arg(revision) AND NOT retired AND state<>'REVOKED' AND NOT refresh_required;
