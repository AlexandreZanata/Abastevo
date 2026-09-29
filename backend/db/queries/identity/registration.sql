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
