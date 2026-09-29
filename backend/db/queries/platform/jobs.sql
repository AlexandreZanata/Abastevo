-- Owned by platform (job queue). Claim paths use SKIP LOCKED so workers
-- never block each other; completion paths bind the fencing token.

-- name: EnqueueJob :one
INSERT INTO job_queue (id, kind, payload, dedupe_key, max_attempts, not_before)
VALUES (@id, @kind, @payload::jsonb, @dedupe_key, @max_attempts, @not_before)
ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
RETURNING id;

-- name: ClaimJob :one
UPDATE job_queue AS j SET
    status = 'leased',
    lease_token = j.lease_token + 1,
    lease_expires_at = now() + (@lease_ttl::text)::interval,
    worker_id = @worker_id,
    attempts = j.attempts + 1
WHERE j.id = (
    SELECT id FROM job_queue
    WHERE (status = 'queued' OR (status = 'leased' AND lease_expires_at < now()))
        AND not_before <= now()
        AND (@kind = '' OR kind = @kind)
    ORDER BY created_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, kind, payload, dedupe_key, status, attempts, max_attempts,
    lease_token, lease_expires_at, worker_id, not_before, last_error, created_at;

-- name: CompleteJob :execrows
UPDATE job_queue
SET status = 'done', lease_expires_at = NULL
WHERE id = @id AND lease_token = @lease_token AND status = 'leased';

-- name: FailJob :one
UPDATE job_queue AS j SET
    status = CASE WHEN j.attempts >= j.max_attempts THEN 'dead' ELSE 'queued' END,
    lease_expires_at = NULL,
    worker_id = '',
    not_before = CASE WHEN j.attempts >= j.max_attempts THEN j.not_before ELSE now() + (@retry_delay::text)::interval END,
    last_error = @last_error
WHERE j.id = @id AND j.lease_token = @lease_token AND j.status = 'leased'
RETURNING status;

-- name: ReplayDeadJob :execrows
UPDATE job_queue
SET status = 'queued', attempts = 0, last_error = @reason,
    not_before = now(), lease_expires_at = NULL, worker_id = ''
WHERE id = @id AND status = 'dead';

-- name: GetJob :one
SELECT id, kind, payload, dedupe_key, status, attempts, max_attempts,
    lease_token, lease_expires_at, worker_id, not_before, last_error, created_at
FROM job_queue
WHERE id = @id;

-- name: GetJobByDedupe :one
SELECT id FROM job_queue WHERE dedupe_key = @dedupe_key;
