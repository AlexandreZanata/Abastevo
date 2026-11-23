-- Sign in with Apple grants and their revocation queue (P13-T05,
-- 000301). Only sealed tokens persist; the queue carries no identity.

-- name: LockAccountForGrant :one
SELECT status FROM accounts WHERE id = @id FOR SHARE;

-- name: UpsertProviderGrant :exec
INSERT INTO account_provider_grants (id, account_id, provider, subject, sealed_token, captured_at)
VALUES (@id, @account_id, @provider, @subject, @sealed_token, @captured_at)
ON CONFLICT (account_id, provider, subject) DO UPDATE
SET id = EXCLUDED.id, sealed_token = EXCLUDED.sealed_token, captured_at = EXCLUDED.captured_at;

-- name: InsertGrantRevocation :exec
INSERT INTO account_provider_revocations (id, provider, sealed_token, requested_at, next_attempt_at)
VALUES (@id, @provider, @sealed_token, @at, @at)
ON CONFLICT (id) DO NOTHING;

-- name: ClaimGrantRevocation :one
WITH candidate AS (
 SELECT r.id FROM account_provider_revocations r
 WHERE r.next_attempt_at <= @at AND (r.lease_until IS NULL OR r.lease_until <= @at)
 ORDER BY r.next_attempt_at, r.id LIMIT 1 FOR UPDATE SKIP LOCKED
)
UPDATE account_provider_revocations r SET lease_nonce = @nonce, lease_until = @lease_until
FROM candidate c WHERE r.id = c.id
RETURNING r.id, r.provider, r.sealed_token, r.requested_at, r.attempts;

-- name: DeleteGrantRevocation :execrows
DELETE FROM account_provider_revocations WHERE id = @id AND lease_nonce = @nonce;

-- name: RetryGrantRevocation :execrows
UPDATE account_provider_revocations
SET attempts = attempts + 1, next_attempt_at = @next_at, lease_nonce = NULL, lease_until = NULL
WHERE id = @id AND lease_nonce = @nonce;
