-- name: DeviceAccount :one
SELECT a.status,COALESCE(a.enterprise_managed_org_id::text,'')::text AS managed_org,
 EXISTS(SELECT 1 FROM payment_billing_exits p WHERE p.account_id=a.id AND p.reason='ACCOUNT_DELETION') AS deletion_pending
FROM accounts a WHERE a.id=CAST(@account_id::text AS uuid) FOR UPDATE;
-- name: DeviceProof :one
SELECT f.access_salt,f.access_hash,f.access_expires,clock_timestamp()::timestamptz AS now
FROM account_session_families f JOIN identity_keys k ON k.id=CAST(@key_id::text AS uuid)
 JOIN identity_contributors c ON c.id=k.contributor_id
 JOIN account_contributor_bindings b ON b.account_id=f.account_id AND b.contributor_id=c.id::text
WHERE f.id=CAST(@family_id::text AS uuid) AND f.account_id=CAST(@account_id::text AS uuid)
 AND k.contributor_id=CAST(@contributor_id::text AS uuid) AND k.fingerprint= @fingerprint::text
 AND f.revoked_at IS NULL AND f.issued_at<=clock_timestamp() AND f.issued_at+interval '30 days'>clock_timestamp() AND f.access_expires>clock_timestamp()
 AND k.revoked_at IS NULL AND c.status='active'
FOR SHARE OF f,k,c,b;
-- name: GetDevice :one
SELECT org_id::text AS org_id,id::text AS id,membership_id::text AS membership_id,key_id::text AS key_id,
 contributor_id::text AS contributor_id,fingerprint,label,state,revision,enrollment_limit,enrolled_at,last_sync_at,revoked_at,revocation_class,
 source_type,COALESCE(source_id::text,'')::text AS source_id,source_revision,created_operation_id::text AS created_operation_id
FROM org_device_enrollments WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);
-- name: DeviceByKey :one
SELECT org_id::text AS org_id,id::text AS id,membership_id::text AS membership_id,key_id::text AS key_id,
 contributor_id::text AS contributor_id,fingerprint,label,state,revision,enrollment_limit,enrolled_at,last_sync_at,revoked_at,revocation_class,
 source_type,COALESCE(source_id::text,'')::text AS source_id,source_revision,created_operation_id::text AS created_operation_id
FROM org_device_enrollments WHERE org_id=CAST(@org_id::text AS uuid) AND key_id=CAST(@key_id::text AS uuid)
 AND(membership_id=CAST(@membership_id::text AS uuid) OR(state='REVOKED' AND revocation_class='LOST_OR_COMPROMISED'))
ORDER BY(revocation_class='LOST_OR_COMPROMISED')DESC,enrolled_at DESC,id LIMIT 1;
-- name: ListDeviceRows :many
SELECT org_id::text AS org_id,id::text AS id,membership_id::text AS membership_id,key_id::text AS key_id,
 contributor_id::text AS contributor_id,fingerprint,label,state,revision,enrollment_limit,enrolled_at,last_sync_at,revoked_at,revocation_class,
 source_type,COALESCE(source_id::text,'')::text AS source_id,source_revision,created_operation_id::text AS created_operation_id
FROM org_device_enrollments WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid)
 AND(@after_id::text='' OR id>NULLIF(@after_id::text,'')::uuid)ORDER BY id LIMIT @row_limit::integer;
-- name: CountEnrolled :one
SELECT count(*)::bigint AS enrolled FROM org_device_enrollments WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid) AND state='ENROLLED';
-- name: InsertDevice :exec
INSERT INTO org_device_enrollments(org_id,id,membership_id,key_id,contributor_id,fingerprint,label,enrollment_limit,created_operation_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@key_id::text AS uuid),CAST(@contributor_id::text AS uuid),@fingerprint::text,@label::text,@enrollment_limit::integer,CAST(@operation_id::text AS uuid));
-- name: RevokeDevice :execrows
UPDATE org_device_enrollments SET state='REVOKED',revision=revision+1,revocation_class= @revocation_class::text,source_type='COMMAND',source_id=CAST(@operation_id::text AS uuid),source_revision=1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint AND state='ENROLLED';
-- name: GetDeviceCommand :one
SELECT actor_id::text AS actor_id,COALESCE(actor_key_id::text,'')::text AS actor_key_id,device_id::text AS device_id,kind,request_hash,reason,result,occurred_at
FROM org_device_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertDeviceCommand :one
INSERT INTO org_device_commands(org_id,operation_id,actor_id,actor_key_id,device_id,kind,request_hash,reason,result)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),NULLIF(@actor_key_id::text,'')::uuid,CAST(@device_id::text AS uuid),@kind::text,@request_hash::text,@reason::text,@result::jsonb)
RETURNING occurred_at;
