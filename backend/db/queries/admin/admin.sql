-- Owned by admin. Sessions keep a deterministic lookup digest plus a salted
-- hash; login counters are locked before evaluation; mutations store only a
-- request hash and their safe outcome. Audit rows are append-only.

-- name: InsertSession :exec
INSERT INTO admin_sessions
    (id, token_lookup, token_salt, token_hash, csrf_salt, csrf_hash,
     key_fingerprint, created_at, last_seen_at, expires_at)
VALUES (@id, @token_lookup, @token_salt, @token_hash, @csrf_salt, @csrf_hash,
    @key_fingerprint, @created_at, @last_seen_at, @expires_at);

-- name: TouchSession :one
UPDATE admin_sessions
SET last_seen_at = @now
WHERE token_lookup = @token_lookup
  AND revoked_at IS NULL
  AND expires_at > @now
  AND last_seen_at > @idle_cutoff
RETURNING *;

-- name: RevokeSession :execrows
UPDATE admin_sessions SET revoked_at = @now
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeAllSessions :execrows
UPDATE admin_sessions SET revoked_at = @now
WHERE revoked_at IS NULL;

-- name: LockLoginWindow :one
INSERT INTO admin_login_attempts (scope, subject, window_start, failures)
VALUES (@scope, @subject, @window_start, 0)
ON CONFLICT (scope, subject, window_start)
DO UPDATE SET failures = admin_login_attempts.failures
RETURNING failures, blocked_until;

-- name: IncrementLoginFailures :one
UPDATE admin_login_attempts
SET failures = failures + 1,
    blocked_until = CASE
        WHEN scope = 'global' AND failures + 1 >= @global_limit THEN @blocked_until
        ELSE blocked_until
    END
WHERE scope = @scope AND subject = @subject AND window_start = @window_start
RETURNING failures, blocked_until;

-- name: BlockLoginWindow :exec
UPDATE admin_login_attempts SET blocked_until = @blocked_until
WHERE scope = @scope AND subject = @subject AND window_start = @window_start;

-- name: InsertAudit :exec
INSERT INTO admin_audit_events
    (session_id, action, target_type, target_id, reason, before, after,
     request_id, client_ip_hash)
VALUES (@session_id, @action, @target_type, @target_id, @reason, NULL, NULL,
    @request_id, @client_ip_hash);

-- name: ReserveMutation :one
INSERT INTO admin_idempotency (session_id, key, action, request_hash)
VALUES (@session_id, @key, @action, @request_hash)
ON CONFLICT (session_id, key) DO NOTHING
RETURNING session_id, key, action, request_hash, response_code, response_body,
    created_at;

-- name: LockMutation :one
SELECT session_id, key, action, request_hash, response_code, response_body,
    created_at
FROM admin_idempotency
WHERE session_id = @session_id AND key = @key
FOR UPDATE;

-- name: CompleteMutation :exec
UPDATE admin_idempotency
SET response_code = @response_code, response_body = @response_body
WHERE session_id = @session_id AND key = @key AND response_code IS NULL;

-- name: DeleteMutation :exec
DELETE FROM admin_idempotency WHERE session_id = @session_id AND key = @key;
