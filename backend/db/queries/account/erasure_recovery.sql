-- name: InsertAccountErasureRequest :exec
INSERT INTO account_erasure_requests(account_id, requested_at, next_attempt_at)
VALUES (@account_id, @at, @at) ON CONFLICT(account_id) DO NOTHING;

-- name: ClaimAccountErasure :one
WITH candidate AS (
 SELECT d.account_id FROM account_erasure_requests d
 WHERE d.completed_at IS NULL AND d.next_attempt_at <= @at
 AND (d.lease_until IS NULL OR d.lease_until <= @at)
 ORDER BY d.next_attempt_at, d.account_id LIMIT 1 FOR UPDATE SKIP LOCKED
)
UPDATE account_erasure_requests r SET lease_nonce = @nonce, lease_until = @lease_until
FROM candidate c WHERE r.account_id = c.account_id RETURNING r.account_id;

-- name: PendingAccountErasureContributor :many
SELECT contributor_id FROM account_erasure_intents
WHERE account_id = @account_id AND completed_at IS NULL
ORDER BY contributor_id LIMIT 1;

-- name: LockAccountErasureLease :one
SELECT account_id FROM account_erasure_requests WHERE account_id = @account_id
AND lease_nonce = @nonce AND lease_until > @at AND completed_at IS NULL FOR UPDATE;

-- name: CompleteAccountErasureContributor :exec
UPDATE account_erasure_intents SET completed_at = @at
WHERE account_id = @account_id AND contributor_id = @contributor_id AND completed_at IS NULL;

-- name: FinishAccountErasure :exec
UPDATE account_erasure_requests r SET lease_nonce = NULL, lease_until = NULL,
next_attempt_at = @next_at,
completed_at = CASE WHEN @succeeded::boolean AND NOT EXISTS (
 SELECT 1 FROM account_erasure_intents i WHERE i.account_id = r.account_id AND i.completed_at IS NULL
) THEN @at::timestamptz ELSE NULL END
WHERE r.account_id = @account_id AND lease_nonce = @nonce AND lease_until > @at;
