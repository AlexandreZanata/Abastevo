-- Company owns its table; account/family joins enforce current source authority.
-- name: LockAccount :one
SELECT id::text FROM accounts WHERE id=CAST(@account_id::text AS uuid) AND status='active' FOR UPDATE;

-- name: LockProof :one
SELECT f.id::text FROM account_session_families f
WHERE f.id=CAST(@family_id::text AS uuid) AND f.account_id=CAST(@account_id::text AS uuid)
  AND f.revoked_at IS NULL AND f.access_hash= @access_hash
  AND f.access_expires= @access_expires AND f.issued_at<= @now::timestamptz
  AND f.issued_at + interval '30 days'> @now::timestamptz
FOR SHARE;

-- name: PruneAccount :exec
DELETE FROM company_web_sessions s WHERE account_id=CAST(@account_id::text AS uuid)
 AND (revoked_at IS NOT NULL OR expires_at<= @now::timestamptz OR last_seen_at<= @idle_cutoff::timestamptz
 OR NOT EXISTS(SELECT 1 FROM account_session_families f WHERE f.id=s.family_id AND f.account_id=s.account_id
   AND f.revoked_at IS NULL AND f.issued_at+interval '30 days'> @now::timestamptz));

-- name: CountAccount :one
SELECT count(*) FROM company_web_sessions WHERE account_id=CAST(@account_id::text AS uuid);

-- name: InsertSession :exec
INSERT INTO company_web_sessions(id,account_id,family_id,token_lookup,token_salt,token_hash,csrf_salt,csrf_hash,
 authenticated_at,created_at,last_seen_at,expires_at)
VALUES(CAST(@id::text AS uuid),CAST(@account_id::text AS uuid),CAST(@family_id::text AS uuid),@token_lookup,@token_salt,@token_hash,@csrf_salt,@csrf_hash,
 @authenticated_at,@created_at,@last_seen_at,@expires_at);

-- name: LockSession :one
SELECT s.id::text,s.account_id::text,s.family_id::text,s.token_lookup,s.token_salt,s.token_hash,s.csrf_salt,s.csrf_hash,
 s.authenticated_at,s.created_at,s.last_seen_at,s.expires_at
FROM company_web_sessions s JOIN accounts a ON a.id=s.account_id
JOIN account_session_families f ON f.id=s.family_id AND f.account_id=s.account_id
WHERE s.token_lookup= @token_lookup AND s.revoked_at IS NULL AND a.status='active' AND f.revoked_at IS NULL
 AND f.issued_at<= @now::timestamptz AND f.issued_at+interval '30 days'> @now::timestamptz
 AND s.created_at<= @now::timestamptz AND s.last_seen_at<= @now::timestamptz AND s.last_seen_at> @idle_cutoff::timestamptz
 AND s.expires_at> @now::timestamptz
FOR UPDATE OF s;

-- name: TouchSession :execrows
UPDATE company_web_sessions s SET last_seen_at= @now::timestamptz
FROM accounts a,account_session_families f
WHERE s.id=CAST(@id::text AS uuid) AND a.id=s.account_id AND a.status='active'
 AND f.id=s.family_id AND f.account_id=s.account_id AND f.revoked_at IS NULL
 AND f.issued_at<= @now::timestamptz AND f.issued_at+interval '30 days'> @now::timestamptz
 AND s.revoked_at IS NULL AND s.last_seen_at<= @now::timestamptz
 AND s.last_seen_at> @idle_cutoff::timestamptz AND s.expires_at> @now::timestamptz;

-- name: RevokeLookup :exec
UPDATE company_web_sessions SET revoked_at= @now::timestamptz WHERE token_lookup= @token_lookup
 AND account_id=CAST(@account_id::text AS uuid) AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE company_web_sessions SET revoked_at= @now::timestamptz
WHERE id=CAST(@id::text AS uuid) AND token_lookup= @token_lookup AND revoked_at IS NULL;

-- name: PurgeExpired :execrows
DELETE FROM company_web_sessions WHERE id IN(
 SELECT s.id FROM company_web_sessions s
 WHERE revoked_at IS NOT NULL OR expires_at<= @now::timestamptz OR last_seen_at<= @idle_cutoff::timestamptz
 OR NOT EXISTS(SELECT 1 FROM accounts a,account_session_families f WHERE a.id=s.account_id AND a.status='active'
   AND f.id=s.family_id AND f.account_id=s.account_id AND f.revoked_at IS NULL
   AND f.issued_at+interval '30 days'> @now::timestamptz)
 ORDER BY expires_at,s.id LIMIT @batch_size FOR UPDATE OF s SKIP LOCKED);

-- name: OldestExpired :one
SELECT min(LEAST(s.expires_at,s.last_seen_at+interval '30 minutes',s.revoked_at,f.revoked_at,
 CASE WHEN a.status<>'active' THEN @now::timestamptz ELSE NULL END))::timestamptz AS oldest
FROM company_web_sessions s JOIN accounts a ON a.id=s.account_id
JOIN account_session_families f ON f.id=s.family_id AND f.account_id=s.account_id
WHERE s.revoked_at IS NOT NULL OR s.expires_at<= @now::timestamptz OR s.last_seen_at<= @idle_cutoff::timestamptz
 OR a.status<>'active' OR f.revoked_at IS NOT NULL OR f.issued_at+interval '30 days'<= @now::timestamptz;
