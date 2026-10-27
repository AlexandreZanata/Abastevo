-- Owned by entitlement. Admin grants are inserted, locked and revoked by
-- primary key or by the active (account, tier) partial unique index. Expiry
-- is evaluated at read time; no job mutates grants.

-- name: LockAccount :one
SELECT id, alias, status, created_at FROM accounts
WHERE id = @id FOR UPDATE;

-- name: LockActiveGrant :one
SELECT id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason
FROM entitlement_grants
WHERE account_id = @account_id AND tier = @tier AND state = 'ACTIVE'
FOR UPDATE;

-- name: InsertGrant :one
INSERT INTO entitlement_grants
    (id, account_id, tier, policy_version, state, start_at, end_at, reason,
     created_session_id)
VALUES (@id, @account_id, @tier, @policy_version, 'ACTIVE', @start_at, @end_at,
    @reason, @created_session_id)
RETURNING id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason;

-- name: RevokeGrant :one
UPDATE entitlement_grants
SET state = 'REVOKED', revoked_at = @now, revoke_reason = @reason
WHERE id = @id AND state = 'ACTIVE'
RETURNING id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason;

-- name: GetGrant :one
SELECT id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason
FROM entitlement_grants WHERE id = @id;

-- name: ActiveGrants :many
SELECT id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason
FROM entitlement_grants
WHERE account_id = @account_id AND state = 'ACTIVE'
  AND start_at <= @now AND (end_at IS NULL OR end_at > @now)
ORDER BY created_at, id;

-- name: ListGrants :many
SELECT id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason
FROM entitlement_grants
WHERE (sqlc.narg('tier')::text IS NULL OR tier = sqlc.narg('tier')::text)
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('before_created')::timestamptz IS NULL OR
       (created_at, id) < (sqlc.narg('before_created')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: AccountGrants :many
SELECT id, account_id, tier, policy_version, state, start_at, end_at, reason,
    created_at, created_session_id, revoked_at, revoke_reason
FROM entitlement_grants
WHERE account_id = @account_id
ORDER BY created_at DESC, id DESC
LIMIT 100;
