-- name: AssignmentAccount :one
SELECT COALESCE(account_id::text,'')::text AS account_id FROM org_memberships
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@membership_id::text AS uuid);

-- name: LockAssignmentAccount :exec
SELECT id FROM accounts WHERE id=CAST(@account_id::text AS uuid) FOR SHARE;

-- name: LockAssignmentRoot :one
SELECT org_id::text AS org_id FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) FOR UPDATE;

-- name: GetAssignment :one
SELECT id::text AS id,membership_id::text AS membership_id,role,scope_type,scope_target::text AS scope_target,
 valid_from,valid_to,assigned_by::text AS assigned_by,reason,revision,ended_at
FROM org_role_assignments WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: InsertAssignment :execrows
INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@membership_id::text AS uuid),@role::text,@scope_type::text,
 CAST(@scope_target::text AS uuid),@valid_from::timestamptz,sqlc.narg(valid_to)::timestamptz,CAST(@assigned_by::text AS uuid),@reason::text)
ON CONFLICT(org_id,id) DO NOTHING;

-- name: EndAssignment :execrows
UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision = @expected_revision::bigint AND ended_at IS NULL;

-- name: CandidateAssignments :many
SELECT r.id::text AS id,r.membership_id::text AS membership_id,r.role,r.scope_type,r.scope_target::text AS scope_target,
 r.valid_from,r.valid_to,r.assigned_by::text AS assigned_by,r.reason,r.revision,r.ended_at
FROM org_role_assignments r JOIN org_memberships m ON (m.org_id,m.id)=(r.org_id,r.membership_id)
JOIN org_employee_identities e ON (e.org_id,e.id)=(m.org_id,m.employee_id) AND e.state='ACTIVE'
JOIN accounts a ON a.id=m.account_id AND a.status='active'
WHERE r.org_id=CAST(@org_id::text AS uuid) AND r.membership_id=CAST(@membership_id::text AS uuid) AND m.state='ACTIVE'
 AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp())
 AND (r.ended_at IS NULL OR r.ended_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM payment_billing_exits x WHERE x.account_id=m.account_id AND x.reason='ACCOUNT_DELETION')
ORDER BY r.id;
