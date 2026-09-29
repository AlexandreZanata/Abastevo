-- Owned by identity. Modules must not import another module's generated
-- package; cross-module reads use owned queries documented in DATA_MODEL.

-- name: CreateChallenge :one
INSERT INTO identity_challenges (id, nonce_hash, fingerprint, purpose, expires_at)
VALUES (@id, @nonce_hash, @fingerprint, @purpose, @expires_at)
RETURNING id;

-- name: ConsumeChallenge :one
UPDATE identity_challenges
SET consumed_at = now()
WHERE id = @id
    AND consumed_at IS NULL
    AND expires_at > now()
RETURNING id, nonce_hash, fingerprint, purpose;

-- name: GetChallenge :one
SELECT id, nonce_hash, fingerprint, purpose, expires_at, consumed_at
FROM identity_challenges
WHERE id = @id;

-- name: CreateContributor :one
INSERT INTO identity_contributors (id)
VALUES (@id)
RETURNING id;

-- name: CreateKey :one
INSERT INTO identity_keys (id, contributor_id, algorithm, public_jwk, fingerprint)
VALUES (@id, @contributor_id, @algorithm, @public_jwk, @fingerprint)
ON CONFLICT (fingerprint) DO NOTHING
RETURNING id, contributor_id;

-- name: FindKeyByFingerprint :one
SELECT id, contributor_id, algorithm, public_jwk, fingerprint, created_at, revoked_at
FROM identity_keys
WHERE fingerprint = @fingerprint;

-- name: GetContributor :one
SELECT id, status, created_at, deleted_at
FROM identity_contributors
WHERE id = @id;

-- name: GetAttributionToken :one
SELECT attribution_token FROM identity_contributors WHERE id = @id;

-- name: SetAttributionToken :execrows
UPDATE identity_contributors
SET attribution_token = @token
WHERE id = @id AND attribution_token IS NULL;

-- name: RevokeKey :execrows
UPDATE identity_keys
SET revoked_at = now()
WHERE id = @id AND revoked_at IS NULL;

-- Erasure (P07-T04, B-BR-016): revoke writes and unlink identity in the
-- narrow privacy workflow. Only these queries may move a contributor to
-- deleted or clear its attribution token; normal writes never touch
-- them.

-- name: DeleteContributor :execrows
UPDATE identity_contributors
SET status = 'deleted', attribution_token = NULL, deleted_at = now()
WHERE id = @id AND status != 'deleted';

-- name: RevokeContributorKeys :execrows
UPDATE identity_keys
SET revoked_at = now()
WHERE contributor_id = @contributor_id AND revoked_at IS NULL;

-- Retention (P07-T05): purge expired challenges in bounded batches.
-- Consumed challenges stay until expiry so replays keep failing
-- closed instead of looking unknown.

-- name: OldestExpiredChallenge :one
SELECT expires_at
FROM identity_challenges
WHERE expires_at < @now
ORDER BY expires_at ASC
LIMIT 1;

-- name: PurgeExpiredChallenges :execrows
DELETE FROM identity_challenges AS dead
WHERE dead.id IN (
    SELECT live.id FROM identity_challenges AS live
    WHERE live.expires_at < @now
    ORDER BY live.expires_at ASC
    LIMIT @batch
);
