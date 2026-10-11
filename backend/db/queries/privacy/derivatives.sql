-- Owned by privacy (CS10B). Training-derivative inventory rows: one
-- per image plus consent revision. The put inserts once and returns
-- no row on a natural-key hit (the caller converges identical
-- retries and conflicts divergent hashes against the stored row).
-- Revocation flips ACTIVE to REVOKED through the guarded transition
-- only. No DELETE path and no touch of sibling tables: revoked
-- receipts stay for the restore filter while physical bytes purge in
-- CS10C.

-- name: PutDerivative :one
INSERT INTO privacy_training_derivatives
    (image_id, consent_revision, contributor_id, contributor_ref,
     source_sha256, derivative_sha256, captured_at, terms_version,
     terms_digest, state, promoted_at, revoked_at)
VALUES (@image_id, @consent_revision, @contributor_id, @contributor_ref,
    @source_sha256, @derivative_sha256, @captured_at, @terms_version,
    @terms_digest, @state, @promoted_at, @revoked_at)
ON CONFLICT DO NOTHING
RETURNING image_id, consent_revision, contributor_id, contributor_ref,
    source_sha256, derivative_sha256, captured_at, terms_version,
    terms_digest, state, promoted_at, revoked_at;

-- name: GetDerivative :one
SELECT image_id, consent_revision, contributor_id, contributor_ref,
    source_sha256, derivative_sha256, captured_at, terms_version,
    terms_digest, state, promoted_at, revoked_at
FROM privacy_training_derivatives
WHERE image_id = @image_id AND consent_revision = @consent_revision;

-- name: ListActiveDerivativesByOwner :many
SELECT d.image_id, d.consent_revision, d.contributor_id, d.contributor_ref,
 d.source_sha256, d.derivative_sha256, d.captured_at, d.terms_version,
 d.terms_digest, d.state, d.promoted_at, d.revoked_at
FROM privacy_training_derivatives d
WHERE d.contributor_id = @contributor_id AND d.state = 'ACTIVE'
AND EXISTS (SELECT 1 FROM privacy_training_consents c WHERE c.contributor_id=d.contributor_id
 AND c.contributor_ref=d.contributor_ref AND c.state='GRANTED' AND c.revision=d.consent_revision
 AND c.terms_version=d.terms_version AND c.terms_digest=d.terms_digest AND d.captured_at>=c.granted_at)
AND NOT EXISTS (SELECT 1 FROM privacy_training_revocations r WHERE r.contributor_id=d.contributor_id AND d.consent_revision<=r.revision)
ORDER BY d.image_id, d.consent_revision;

-- name: MarkDerivativeRevoked :one
UPDATE privacy_training_derivatives
SET state = 'REVOKED', revoked_at = @revoked_at
WHERE image_id = @image_id AND consent_revision = @consent_revision
  AND state = 'ACTIVE'
RETURNING image_id;

-- name: ListRevokedDerivativesBefore :many
SELECT image_id, consent_revision, contributor_id, contributor_ref,
    source_sha256, derivative_sha256, captured_at, terms_version,
    terms_digest, state, promoted_at, revoked_at
FROM privacy_training_derivatives
WHERE state = 'REVOKED' AND revoked_at < @before
ORDER BY revoked_at, image_id, consent_revision;

-- name: ListDerivativesByOwner :many
SELECT image_id, consent_revision, contributor_id, contributor_ref,
    source_sha256, derivative_sha256, captured_at, terms_version,
    terms_digest, state, promoted_at, revoked_at
FROM privacy_training_derivatives
WHERE contributor_id = @contributor_id
ORDER BY image_id, consent_revision;
