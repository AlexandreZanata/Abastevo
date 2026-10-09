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
     policy_version, photo_capture_id)
VALUES (@id, @contributor_ref, @client_session_id, @mime, @declared_bytes,
    @claimed_sha256, @quarantine_key, @status, @created_at, @expires_at,
    @policy_version, @photo_capture_id)
ON CONFLICT (contributor_ref, client_session_id) DO NOTHING
RETURNING id;

-- name: GetSessionByNaturalKey :one
SELECT id, contributor_ref, client_session_id, mime, declared_bytes,
    claimed_sha256, quarantine_key, status, status_reason, created_at,
    expires_at, updated_at, policy_version, photo_capture_id
FROM evidence_sessions
WHERE contributor_ref = @contributor_ref AND client_session_id = @client_session_id;

-- name: GetSession :one
SELECT id, contributor_ref, client_session_id, mime, declared_bytes,
    claimed_sha256, quarantine_key, status, status_reason, created_at,
    expires_at, updated_at, policy_version, photo_capture_id
FROM evidence_sessions
WHERE id = @id;

-- name: ClaimVerifying :execrows
UPDATE evidence_sessions
SET status = 'VERIFYING', updated_at = now()
WHERE id = @id AND status = 'ISSUED' AND (photo_capture_id IS NULL OR expires_at > now());

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
SELECT id, final_key, created_at, received_at, retention_extended_until
FROM evidence_objects
WHERE final_deleted_at IS NULL
    AND COALESCE(received_at, created_at) <= @young_cutoff
ORDER BY COALESCE(received_at, created_at) ASC
LIMIT @batch;

-- OverdueFinals lists copies past their 24 h deadline whose bytes may
-- still be present: the failing-policy signal for the intake circuit
-- breaker and the overdue audit (P15-T04, B-BR-M04). Bounded, newest
-- deadline first is unnecessary; oldest first surfaces the worst lag.

-- name: OverdueFinals :many
SELECT id, final_key, created_at, received_at
FROM evidence_objects
WHERE final_deleted_at IS NULL
    AND COALESCE(received_at, created_at) < @cutoff
ORDER BY COALESCE(received_at, created_at) ASC
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
     width, height, dhash, created_at, received_at)
VALUES (@id, @session_id, @final_key, @source_sha256, @sanitized_sha256,
    @width, @height, @dhash, @created_at, @received_at)
RETURNING id;

-- name: GetObject :one
SELECT o.id, o.session_id, o.final_key, o.source_sha256,
    o.sanitized_sha256, o.width, o.height, o.dhash,
    o.bound_capture_id, o.bound_observation_id, o.created_at, o.received_at, o.final_deleted_at,
    s.contributor_ref, s.status, s.expires_at, s.photo_capture_id
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.id = @id;

-- name: GetObjectByFinalKey :one
SELECT id, session_id, final_key, source_sha256, sanitized_sha256,
    width, height, dhash, created_at
FROM evidence_objects
WHERE final_key = @final_key;

-- name: GetObjectBySession :one
SELECT o.id, o.session_id, o.final_key, o.source_sha256,
    o.sanitized_sha256, o.width, o.height, o.dhash,
    o.bound_capture_id, o.bound_observation_id, o.created_at, o.final_deleted_at,
    s.contributor_ref, s.status, s.expires_at, s.photo_capture_id
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE o.session_id = @session_id;

-- name: ClaimObjectBinding :execrows
UPDATE evidence_objects
SET bound_observation_id = @observation_id
WHERE id = @id AND bound_capture_id IS NULL AND (bound_observation_id IS NULL OR bound_observation_id = @observation_id);

-- Erasure inventory (P07-T04, B-BR-016): owner sessions and objects for
-- purge. Keys travel only into the storage-delete port, never into
-- exports or logs.

-- name: ListSessionsByContributor :many
SELECT id, quarantine_key, status, created_at
FROM evidence_sessions
WHERE contributor_ref = @contributor_ref
ORDER BY created_at ASC
LIMIT @page_limit;

-- name: ListObjectsByContributor :many
SELECT o.id, o.session_id, o.final_key, o.created_at
FROM evidence_objects o
JOIN evidence_sessions s ON s.id = o.session_id
WHERE s.contributor_ref = @contributor_ref
ORDER BY o.created_at ASC
LIMIT @page_limit;

-- name: ExpireSessionsByContributor :execrows
UPDATE evidence_sessions
SET status = 'EXPIRED', updated_at = now()
WHERE contributor_ref = @contributor_ref AND status != 'EXPIRED';

-- name: ClaimPhotoObjectBinding :execrows
UPDATE evidence_objects o SET bound_capture_id = @capture_id
FROM evidence_sessions s
WHERE o.id = @id AND s.id = o.session_id AND s.contributor_ref = @contributor_ref
AND s.photo_capture_id = @capture_id AND s.status = 'READY' AND s.expires_at > @now_at
AND o.final_deleted_at IS NULL AND o.bound_observation_id IS NULL
AND (o.bound_capture_id IS NULL OR o.bound_capture_id = @capture_id);
