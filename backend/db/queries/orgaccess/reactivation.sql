-- Data candidates only; normal AUTH still requires ACTIVE membership.
-- name: ReactivationCandidateAssignments :many
SELECT r.id::text AS id,r.membership_id::text AS membership_id,r.role,r.scope_type,r.scope_target::text AS scope_target,
 r.valid_from,r.valid_to,r.assigned_by::text AS assigned_by,r.reason,r.revision,r.ended_at
FROM org_role_assignments r JOIN org_memberships m ON (m.org_id,m.id)=(r.org_id,r.membership_id)
JOIN org_employee_identities e ON (e.org_id,e.id)=(m.org_id,m.employee_id) AND e.state='ACTIVE'
JOIN accounts a ON a.id=m.account_id AND a.status='active'
WHERE r.org_id=CAST(@org_id::text AS uuid) AND r.membership_id=CAST(@membership_id::text AS uuid) AND m.state IN('ACTIVE','SUSPENDED')
 AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp())
 AND (r.ended_at IS NULL OR r.ended_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM payment_billing_exits x WHERE x.account_id=m.account_id AND x.reason='ACCOUNT_DELETION')
ORDER BY r.id;
