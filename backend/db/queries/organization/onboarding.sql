-- name: LockOnboardingAccount :one
SELECT status,(enterprise_managed_org_id IS NOT NULL)::boolean AS managed,
 EXISTS(SELECT 1 FROM payment_billing_exits WHERE account_id=accounts.id AND reason='ACCOUNT_DELETION') AS deletion_pending
FROM accounts WHERE id=CAST(@account_id::text AS uuid) FOR UPDATE;

-- name: CurrentOnboardingTime :one
SELECT clock_timestamp()::timestamptz AS now;

-- name: LockOnboardingProof :one
SELECT c.id::text AS id FROM company_web_sessions c
JOIN account_session_families f ON (f.account_id,f.id)=(c.account_id,c.family_id)
WHERE c.id=CAST(@session_id::text AS uuid) AND c.account_id=CAST(@account_id::text AS uuid)
 AND c.revoked_at IS NULL AND f.revoked_at IS NULL
 AND c.created_at<=clock_timestamp() AND c.last_seen_at<=clock_timestamp()
 AND c.last_seen_at>clock_timestamp()-interval '30 minutes' AND c.expires_at>clock_timestamp()
 AND c.authenticated_at<=clock_timestamp() AND c.authenticated_at>clock_timestamp()-interval '15 minutes'
 AND f.issued_at<=clock_timestamp() AND f.issued_at>clock_timestamp()-interval '30 days'
FOR SHARE OF c,f;

-- name: InstallCreationAccount :exec
SELECT set_config('anpfuel.creation_account',@account_id::text,true);

-- name: CreationReceipt :one
SELECT org_id::text AS org_id,unit_id::text AS unit_id,employee_id::text AS employee_id,
 membership_id::text AS membership_id,assignment_id::text AS assignment_id,request_hash,result_revision
FROM org_onboarding_commands WHERE actor_id=CAST(@account_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid) AND kind='CREATED';

-- name: InsertDefaultUnit :exec
INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@name::text,'MAIN',@time_zone::text,'ACTIVE');

-- name: InsertCreationReceipt :exec
INSERT INTO org_onboarding_commands(org_id,operation_id,kind,actor_id,request_hash,unit_id,employee_id,membership_id,assignment_id,terms_version,terms_digest,terms_kind,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),'CREATED',CAST(@actor_id::text AS uuid),@request_hash::text,
 CAST(@unit_id::text AS uuid),CAST(@employee_id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@assignment_id::text AS uuid),@terms_version::text,@terms_digest::text,'SYNTHETIC',1);

-- name: ActivationReceipt :one
SELECT actor_id::text AS actor_id,request_hash,result_revision FROM org_onboarding_commands
WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid) AND kind='ACTIVATED';

-- name: LockSetupRoot :exec
SELECT org_id FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) FOR UPDATE;

-- name: SetupFacts :one
SELECT (SELECT count(*) FROM org_units WHERE org_id=CAST(@org_id::text AS uuid) AND state='ACTIVE')::bigint AS units,
 (SELECT count(DISTINCT m.id) FROM org_memberships m JOIN org_role_assignments r ON (r.org_id,r.membership_id)=(m.org_id,m.id)
  JOIN accounts a ON a.id=m.account_id AND a.status='active'
  WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.state='ACTIVE' AND r.role='OWNER' AND r.scope_type='ORG'
   AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp()) AND r.ended_at IS NULL
   AND NOT EXISTS(SELECT 1 FROM payment_billing_exits x WHERE x.account_id=m.account_id AND x.reason='ACCOUNT_DELETION'))::bigint AS owners,
 EXISTS(SELECT 1 FROM org_onboarding_commands WHERE org_id=CAST(@org_id::text AS uuid) AND kind='CREATED'
  AND terms_version = @terms_version::text AND terms_digest = @terms_digest::text AND terms_kind='SYNTHETIC') AS terms;

-- name: InsertActivationReceipt :exec
INSERT INTO org_onboarding_commands(org_id,operation_id,kind,actor_id,request_hash,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),'ACTIVATED',CAST(@actor_id::text AS uuid),@request_hash::text,@revision::bigint);
