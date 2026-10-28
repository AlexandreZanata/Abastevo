-- Owned by staff. Roles are code-frozen; only assignments are stored. Expiry
-- is evaluated at read time and account deletion revokes active assignments.

-- name: LockAccount :one
SELECT id, alias, status, created_at FROM accounts WHERE id = @id FOR UPDATE;

-- name: GetActiveAssignment :one
SELECT * FROM staff_assignments
WHERE account_id = @account_id AND role = @role AND state = 'ACTIVE'
FOR UPDATE;

-- name: InsertAssignment :one
INSERT INTO staff_assignments
    (id, account_id, role, state, start_at, end_at, reason, created_session_id)
VALUES (@id, @account_id, @role, 'ACTIVE', @start_at, @end_at, @reason, @created_session_id)
RETURNING *;

-- name: GetAssignment :one
SELECT * FROM staff_assignments WHERE id = @id;

-- name: RevokeAssignment :one
UPDATE staff_assignments
SET state = 'REVOKED', revoked_at = @now, revoke_reason = @reason
WHERE id = @id AND state = 'ACTIVE'
RETURNING *;

-- name: ActiveAssignments :many
SELECT * FROM staff_assignments
WHERE account_id = @account_id AND state = 'ACTIVE'
  AND start_at <= @now AND (end_at IS NULL OR end_at > @now)
ORDER BY role;

-- name: ListAssignmentsByAccount :many
SELECT * FROM staff_assignments
WHERE account_id = @account_id
ORDER BY created_at DESC, id DESC
LIMIT 100;
