-- name: TransferMemberAccount :one
SELECT COALESCE(account_id::text,'')::text AS account_id FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@member_id::text AS uuid);

-- name: TransferMemberFacts :one
SELECT m.state,m.access_model,e.state AS employee_state,COALESCE(a.id::text,'')::text AS account_id,COALESCE(a.status,'')::text AS account_status,
 COALESCE(a.enterprise_managed_org_id::text,'')::text AS managed_org,
 EXISTS(SELECT 1 FROM payment_billing_exits x WHERE x.account_id=a.id AND x.reason='ACCOUNT_DELETION') AS deletion_pending
FROM org_memberships m JOIN org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id) LEFT JOIN accounts a ON a.id=m.account_id
WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.id=CAST(@member_id::text AS uuid);

-- name: TransferCurrentOwner :one
SELECT EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@member_id::text AS uuid)
 AND role='OWNER' AND scope_type='ORG' AND scope_target=org_id AND ended_at IS NULL AND valid_from<=clock_timestamp() AND(valid_to IS NULL OR valid_to>clock_timestamp())) AS is_owner;

-- name: GetTransferSnapshot :one
SELECT transfer_id::text AS transfer_id,org_id::text AS org_id,initiator_membership_id::text AS initiator_membership_id,target_membership_id::text AS target_membership_id,
 step_down_to_admin,state,expires_at,accepted_at,cancelled_at,declined_at,created_at,operation_id::text AS operation_id
FROM org_ownership_transfers WHERE org_id=CAST(@org_id::text AS uuid)
 AND((@transfer_id::text='' AND state='PENDING') OR transfer_id=CAST(NULLIF(@transfer_id::text,'') AS uuid)) LIMIT 1;

-- name: GetOwnershipCommand :one
SELECT actor_account_id::text AS actor_account_id,kind,request_hash,result FROM org_ownership_commands
WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertOwnershipCommand :exec
INSERT INTO org_ownership_commands(org_id,operation_id,actor_account_id,kind,request_hash,transfer_id,result)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),@kind::text,@request_hash::text,CAST(@transfer_id::text AS uuid),@result::jsonb);
