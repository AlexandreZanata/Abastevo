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

-- name: ExpireStuckSession :execrows
UPDATE evidence_sessions
SET status = 'EXPIRED', updated_at = now()
WHERE id = @id AND status = 'VERIFYING' AND created_at <= @cutoff;

-- name: CountSessionsSince :one
SELECT count(*) FROM evidence_sessions
WHERE contributor_ref = @contributor_ref AND created_at >= @since;

-- Sweep inventory (P05-T05). State transitions stay conditional; every
-- deletion marker is set-if-unset so repeated runs converge. Postgres
-- UPDATE takes no LIMIT, so batched transitions select their keys in a
-- subquery instead.

-- name: ExpireIdleSessions :many
UPDATE evidence_sessions AS s
SET status = 'EXPIRED', updated_at = now()
WHERE s.id IN (
    SELECT inner_s.id FROM evidence_sessions AS inner_s
    WHERE inner_s.status = 'ISSUED' AND inner_s.expires_at <= @now
    ORDER BY inner_s.expires_at ASC
    LIMIT @batch
)
RETURNING s.id, s.quarantine_key;

-- name: ListStuckVerifying :many
SELECT id, quarantine_key, created_at FROM evidence_sessions
WHERE status = 'VERIFYING' AND created_at <= @cutoff
ORDER BY created_at ASC
LIMIT @batch;

-- name: ListYoungVerifying :many
SELECT id, quarantine_key, created_at FROM evidence_sessions
WHERE status = 'VERIFYING' AND created_at > @cutoff
ORDER BY created_at ASC
LIMIT @batch;

-- name: MarkQuarantineDeleted :execrows
UPDATE evidence_sessions
SET quarantine_deleted_at = now()
WHERE id = @id AND quarantine_deleted_at IS NULL;

-- name: QuarantineCandidates :many
SELECT id, quarantine_key, status, created_at FROM evidence_sessions
WHERE quarantine_deleted_at IS NULL
    AND (status IN ('READY', 'REJECTED', 'EXPIRED') OR created_at <= @old_cutoff)
ORDER BY created_at ASC
LIMIT @batch;

-- name: StaleFinalCandidates :many
SELECT id, final_key, created_at, retention_extended_until
FROM evidence_objects
WHERE final_deleted_at IS NULL AND created_at <= @young_cutoff
ORDER BY created_at ASC
LIMIT @batch;

-- name: MarkFinalDeleted :execrows
UPDATE evidence_objects
SET final_deleted_at = now()
WHERE id = @id AND final_deleted_at IS NULL;

-- name: PurgeObjectsBefore :execrows
DELETE FROM evidence_objects
WHERE created_at < @cutoff;

-- name: FindByDHash :many
SELECT id, session_id, final_key, dhash, created_at
FROM evidence_objects
WHERE dhash = @dhash AND created_at >= @since
ORDER BY created_at ASC;

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
    o.bound_observation_id, o.created_at, o.final_deleted_at,
    s.contributor_ref, s.status
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.id = @id;

-- name: GetObjectBySession :one
SELECT o.id, o.session_id, o.final_key, o.source_sha256,
    o.sanitized_sha256, o.width, o.height, o.dhash,
    o.bound_observation_id, o.created_at, o.final_deleted_at,
    s.contributor_ref, s.status
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.session_id = @session_id;

-- name: ClaimObjectBinding :execrows
UPDATE evidence_objects
SET bound_observation_id = @observation_id
WHERE id = @id AND (bound_observation_id IS NULL OR bound_observation_id = @observation_id);
