-- name: ListAccountContextRefs :many
SELECT org_id::text AS org_id,id::text AS membership_id
FROM org_memberships
WHERE account_id=CAST(@account_id::text AS uuid) AND state='ACTIVE' AND access_model='EXISTING_ACCOUNT'
 AND (NULLIF(@after_org_id::text,'') IS NULL OR org_id>CAST(NULLIF(@after_org_id::text,'') AS uuid))
ORDER BY org_id LIMIT @row_limit::int;

-- name: ContextProof :one
SELECT (c.authenticated_at<=clock_timestamp() AND c.authenticated_at>clock_timestamp()-interval '15 minutes')::boolean AS fresh
FROM company_web_sessions c
JOIN account_session_families f ON (f.account_id,f.id)=(c.account_id,c.family_id)
WHERE c.id=CAST(@session_id::text AS uuid) AND c.account_id=CAST(@account_id::text AS uuid)
 AND c.revoked_at IS NULL AND f.revoked_at IS NULL
 AND c.created_at<=clock_timestamp() AND c.last_seen_at<=clock_timestamp()
 AND c.last_seen_at>clock_timestamp()-interval '30 minutes' AND c.expires_at>clock_timestamp()
 AND c.authenticated_at<=clock_timestamp()
 AND f.issued_at<=clock_timestamp() AND f.issued_at>clock_timestamp()-interval '30 days'
FOR SHARE OF c,f;
