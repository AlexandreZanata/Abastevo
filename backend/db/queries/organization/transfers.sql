-- name: GetPendingTransferForOrg :one
SELECT transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, accepted_at, cancelled_at, declined_at, created_at, operation_id
FROM org_ownership_transfers
WHERE org_id = $1 AND state = 'PENDING'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetTransferByID :one
SELECT transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, accepted_at, cancelled_at, declined_at, created_at, operation_id
FROM org_ownership_transfers
WHERE org_id = $1 AND transfer_id = $2;

-- name: GetTransferByOperationID :one
SELECT transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, accepted_at, cancelled_at, declined_at, created_at, operation_id
FROM org_ownership_transfers
WHERE org_id = $1 AND operation_id = $2;

-- name: CreateTransfer :one
INSERT INTO org_ownership_transfers (
    transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, created_at, operation_id
) VALUES (
    $1, $2, $3, $4, $5, 'PENDING', $6, $7, $8
)
RETURNING transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, accepted_at, cancelled_at, declined_at, created_at, operation_id;

-- name: LockTransferForUpdate :one
SELECT transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, operation_id
FROM org_ownership_transfers
WHERE org_id = $1 AND transfer_id = $2
FOR UPDATE;

-- name: UpdateTransferState :one
UPDATE org_ownership_transfers
SET state = $3,
    accepted_at = CASE WHEN $3 = 'ACCEPTED' THEN clock_timestamp() ELSE accepted_at END,
    cancelled_at = CASE WHEN $3 = 'CANCELLED' THEN clock_timestamp() ELSE cancelled_at END,
    declined_at = CASE WHEN $3 = 'DECLINED' THEN clock_timestamp() ELSE declined_at END
WHERE org_id = $1 AND transfer_id = $2 AND state = 'PENDING'
RETURNING transfer_id, org_id, initiator_membership_id, target_membership_id, step_down_to_admin, state, expires_at, accepted_at, cancelled_at, declined_at, created_at, operation_id;

-- name: CheckTransferTarget :one
SELECT m.id AS membership_id, m.state AS membership_state, e.state AS employee_state, a.id AS account_id, a.enterprise_managed_org_id
FROM org_memberships m
JOIN org_employee_identities e ON e.org_id = m.org_id AND e.id = m.employee_id
JOIN accounts a ON a.id = m.account_id
WHERE m.org_id = $1 AND m.id = $2;

-- name: CheckTargetIsOwner :one
SELECT EXISTS(
    SELECT 1 FROM org_role_assignments
    WHERE org_id = $1 AND membership_id = $2 AND role = 'OWNER'
      AND ended_at IS NULL AND (valid_to IS NULL OR valid_to > clock_timestamp())
) AS is_owner;

-- name: ExpireTransfers :exec
UPDATE org_ownership_transfers
SET state = 'EXPIRED'
WHERE org_id = $1 AND state = 'PENDING' AND expires_at <= $2;
