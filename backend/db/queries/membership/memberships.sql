-- name: GetEmployee :one
SELECT id::text AS id,state,revision,COALESCE(merged_into::text,'')::text AS merged_into
FROM org_employee_identities WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: InsertEmployee :execrows
INSERT INTO org_employee_identities(org_id,id,state,revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),'ACTIVE',1)
ON CONFLICT(org_id,id) DO NOTHING;

-- name: GetMembership :one
SELECT id::text AS id,employee_id::text AS employee_id,
 COALESCE(account_id::text,'')::text AS account_id,access_model,state,revision,suspension_origin,end_reason,
 COALESCE(account_tombstone::text,'')::text AS account_tombstone
FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: ActiveMembershipID :one
SELECT m.id::text AS id FROM org_memberships m
JOIN accounts a ON a.id=m.account_id AND a.status='active'
WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.account_id=CAST(@account_id::text AS uuid) AND m.state='ACTIVE'
AND NOT EXISTS(SELECT 1 FROM payment_billing_exits e WHERE e.account_id=m.account_id AND e.reason='ACCOUNT_DELETION');

-- name: InsertMembership :execrows
INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state,revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@employee_id::text AS uuid),
 NULLIF(@account_id::text,'')::uuid,@access_model::text,'INVITED',1)
ON CONFLICT(org_id,id) DO NOTHING;

-- name: AdvanceMembership :execrows
UPDATE org_memberships SET account_id=NULLIF(@account_id::text,'')::uuid,state = @state::text,
 revision = @revision::bigint,suspension_origin = @suspension_origin::text,end_reason = @end_reason::text
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid)
 AND revision = @expected_revision::bigint AND state<>'ENDED';
