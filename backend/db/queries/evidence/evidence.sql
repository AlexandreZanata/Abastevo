-- Owned by evidence. Sessions mutate only through guarded conditional
-- updates on the expected state (callers reload and converge on zero
-- matched rows); objects are insert-only, and binding claims exactly one
-- observation with set-if-unbound-or-same semantics. No UPDATE or DELETE
-- against evidence_objects except the single-column bind claim, and no
-- DELETE anywhere in this file.

-- name: InsertSession :one
INSERT INTO evidence_sessions
    (id, contributor_ref, client_session_id, mime, declared_bytes,
     claimed_sha256, quarantine_key, status, created_at, expires_at,
     policy_version)
VALUES (@id, @contributor_ref, @client_session_id, @mime, @declared_bytes,
    @claimed_sha256, @quarantine_key, @status, @created_at, @expires_at,
    @policy_version)
ON CONFLICT (contributor_ref, client_session_id) DO NOTHING
RETURNING id;

-- name: GetSessionByNaturalKey :one
SELECT id, contributor_ref, client_session_id, mime, declared_bytes,
    claimed_sha256, quarantine_key, status, status_reason, created_at,
    expires_at, updated_at, policy_version
FROM evidence_sessions
WHERE contributor_ref = @contributor_ref AND client_session_id = @client_session_id;

-- name: GetSession :one
SELECT id, contributor_ref, client_session_id, mime, declared_bytes,
    claimed_sha256, quarantine_key, status, status_reason, created_at,
    expires_at, updated_at, policy_version
FROM evidence_sessions
WHERE id = @id;

-- name: ClaimVerifying :execrows
UPDATE evidence_sessions
SET status = 'VERIFYING', updated_at = now()
WHERE id = @id AND status = 'ISSUED';

-- name: MarkSessionReady :execrows
UPDATE evidence_sessions
SET status = 'READY', updated_at = now()
WHERE id = @id AND status = 'VERIFYING';

-- name: MarkSessionRejected :execrows
UPDATE evidence_sessions
SET status = 'REJECTED', status_reason = @reason_codes, updated_at = now()
WHERE id = @id AND status = 'VERIFYING';

-- name: MarkSessionExpired :execrows
UPDATE evidence_sessions
SET status = 'EXPIRED', updated_at = now()
WHERE id = @id AND status = 'ISSUED';

-- name: CountSessionsSince :one
SELECT count(*) FROM evidence_sessions
WHERE contributor_ref = @contributor_ref AND created_at >= @since;

-- name: InsertObject :one
INSERT INTO evidence_objects
    (id, session_id, final_key, source_sha256, sanitized_sha256,
     width, height, dhash, created_at)
VALUES (@id, @session_id, @final_key, @source_sha256, @sanitized_sha256,
    @width, @height, @dhash, @created_at)
RETURNING id;

-- name: GetObject :one
SELECT o.id, o.session_id, o.final_key, o.source_sha256,
    o.sanitized_sha256, o.width, o.height, o.dhash,
    o.bound_observation_id, o.created_at,
    s.contributor_ref, s.status
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.id = @id;

-- name: GetObjectBySession :one
SELECT o.id, o.session_id, o.final_key, o.source_sha256,
    o.sanitized_sha256, o.width, o.height, o.dhash,
    o.bound_observation_id, o.created_at,
    s.contributor_ref, s.status
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.session_id = @session_id;

-- name: ClaimObjectBinding :execrows
UPDATE evidence_objects
SET bound_observation_id = @observation_id
WHERE id = @id AND (bound_observation_id IS NULL OR bound_observation_id = @observation_id);
