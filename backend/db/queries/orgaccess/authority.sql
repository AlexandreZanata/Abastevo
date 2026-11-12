-- name: CurrentAuthority :many
SELECT m.account_id::text AS account_id,m.id::text AS membership_id,m.employee_id::text AS employee_id,
 m.state AS membership_state,m.access_model,e.state AS employee_state,o.state AS organization_state,
 COALESCE(r.id::text,'')::text AS assignment_id,COALESCE(r.role,'')::text AS role,
 COALESCE(r.scope_type,'')::text AS scope_type,COALESCE(r.scope_target::text,'')::text AS scope_target,
 r.valid_from,r.valid_to,COALESCE(r.assigned_by::text,'')::text AS assigned_by,
 COALESCE(r.reason,'')::text AS reason,COALESCE(r.revision,0)::bigint AS revision,r.ended_at
FROM org_memberships m
JOIN organizations o ON o.org_id=m.org_id AND o.has_snapshot
JOIN org_employee_identities e ON (e.org_id,e.id)=(m.org_id,m.employee_id)
JOIN accounts a ON a.id=m.account_id AND a.status='active'
LEFT JOIN org_role_assignments r ON (r.org_id,r.membership_id)=(m.org_id,m.id)
 AND r.valid_from<=statement_timestamp() AND (r.valid_to IS NULL OR r.valid_to>statement_timestamp())
 AND (r.ended_at IS NULL OR r.ended_at>statement_timestamp())
WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.account_id=CAST(@account_id::text AS uuid) AND m.state<>'ENDED'
 AND NOT EXISTS(SELECT 1 FROM payment_billing_exits x WHERE x.account_id=m.account_id AND x.reason='ACCOUNT_DELETION')
ORDER BY r.id;
