-- Owned by identity. Retries replay the stored outcome; a changed body for
-- the same key conflicts instead of executing twice.

-- name: ReserveAttempt :one
INSERT INTO identity_idempotency
    (contributor_id, method, route, key, request_hash, expires_at)
VALUES (@contributor_id, @method, @route, @key, @request_hash, @expires_at)
ON CONFLICT (contributor_id, method, route, key) DO NOTHING
RETURNING contributor_id, method, route, key, request_hash, response_code,
    response_json, created_at, expires_at;

-- name: LockAttempt :one
SELECT contributor_id, method, route, key, request_hash, response_code,
    response_json, created_at, expires_at
FROM identity_idempotency
WHERE contributor_id = @contributor_id AND method = @method
    AND route = @route AND key = @key
FOR UPDATE;

-- name: CompleteAttempt :exec
UPDATE identity_idempotency
SET response_code = @response_code, response_json = @response_json::jsonb
WHERE contributor_id = @contributor_id AND method = @method
    AND route = @route AND key = @key AND response_code IS NULL;

-- name: DeleteAttempt :exec
DELETE FROM identity_idempotency
WHERE contributor_id = @contributor_id AND method = @method
    AND route = @route AND key = @key;

-- Retention (P07-T05): purge expired idempotency windows in bounded
-- batches. Expired outcomes are never replayed, so removal only frees
-- space; seven-day mobile retries use fresh keys.

-- name: OldestExpiredAttempt :one
SELECT expires_at
FROM identity_idempotency
WHERE expires_at < @now
ORDER BY expires_at ASC
LIMIT 1;

-- name: PurgeExpiredAttempts :execrows
DELETE FROM identity_idempotency AS dead
WHERE (dead.contributor_id, dead.method, dead.route, dead.key) IN (
    SELECT live.contributor_id, live.method, live.route, live.key
    FROM identity_idempotency AS live
    WHERE live.expires_at < @now
    ORDER BY live.expires_at ASC
    LIMIT @batch
);
