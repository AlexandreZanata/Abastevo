-- name: ReadOrganizationClosure :one
SELECT operation_id::text AS operation_id,closes_at
FROM org_closure_requests WHERE org_id=sqlc.arg('org_id')::text::uuid AND state='PENDING';

-- name: InsertOrganizationClosure :exec
INSERT INTO org_closure_requests(org_id,operation_id,job_id,archive_revision,requested_at,closes_at)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('job_id')::text::uuid,
       sqlc.arg('archive_revision')::bigint,sqlc.arg('requested_at')::timestamptz,sqlc.arg('closes_at')::timestamptz);

-- name: CancelOrganizationClosure :execrows
UPDATE org_closure_requests SET state='CANCELLED',cancelled_by_operation=sqlc.arg('cancel_operation')::text::uuid
WHERE org_id=sqlc.arg('org_id')::text::uuid AND operation_id=sqlc.arg('operation_id')::text::uuid AND state='PENDING'
  AND closes_at>clock_timestamp();

-- name: FindScheduledOrganizationClosure :one
SELECT state,closes_at FROM org_closure_requests WHERE org_id=sqlc.arg('org_id')::text::uuid
 AND operation_id=sqlc.arg('operation_id')::text::uuid AND job_id=sqlc.arg('job_id')::text::uuid;

-- name: CompleteOrganizationClosure :execrows
UPDATE org_closure_requests c SET state='COMPLETED',hold_count=sqlc.arg('hold_count')::bigint
FROM job_queue j WHERE c.org_id=sqlc.arg('org_id')::text::uuid AND c.operation_id=sqlc.arg('operation_id')::text::uuid
  AND c.job_id=sqlc.arg('job_id')::text::uuid AND c.state='PENDING' AND c.closes_at<=clock_timestamp()
  AND j.id=c.job_id AND j.status='leased' AND j.lease_token=sqlc.arg('lease_token')::bigint AND j.lease_expires_at>clock_timestamp();

-- name: InsertOrganizationLifecycleReceipt :exec
INSERT INTO org_lifecycle_receipts(org_id,operation_id,result)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('result')::jsonb);

-- name: ReadOrganizationLifecycleReceipt :one
SELECT result FROM org_lifecycle_receipts WHERE org_id=sqlc.arg('org_id')::text::uuid AND operation_id=sqlc.arg('operation_id')::text::uuid;

-- name: LockOrganizationClosureAccounts :many
SELECT a.id::text FROM accounts a
WHERE EXISTS (SELECT 1 FROM org_memberships m WHERE m.org_id=sqlc.arg('org_id')::text::uuid AND m.account_id=a.id AND m.state<>'ENDED')
ORDER BY a.id FOR SHARE;

-- name: EndOrganizationMemberships :exec
UPDATE org_memberships SET state='ENDED',end_reason='ORGANIZATION_CLOSED',revision=revision+1
WHERE org_id=sqlc.arg('org_id')::text::uuid AND state<>'ENDED';

-- name: OrganizationClosureWindowElapsed :one
SELECT EXISTS (SELECT 1 FROM org_closure_requests WHERE org_id=sqlc.arg('org_id')::text::uuid
               AND state='PENDING' AND closes_at<=statement_timestamp())::boolean AS elapsed;

-- name: OrganizationClosureOwners :many
SELECT m.id::text AS membership_id FROM org_memberships m
JOIN accounts a ON a.id=m.account_id AND a.status='active' AND a.enterprise_managed_org_id IS NULL
WHERE m.org_id=sqlc.arg('org_id')::text::uuid AND m.state='ACTIVE'
 AND EXISTS (SELECT 1 FROM org_role_assignments r WHERE (r.org_id,r.membership_id)=(m.org_id,m.id)
    AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=m.org_id AND r.ended_at IS NULL
    AND r.valid_from<=statement_timestamp() AND (r.valid_to IS NULL OR r.valid_to>statement_timestamp()))
ORDER BY m.id;
