-- Owner serialization never surrounds network/image processing.
-- name: LockTrainingOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended('privacy-training:' || @contributor_id::text, 0));

-- name: GetTrainingRevocation :one
SELECT revision, revoked_at FROM privacy_training_revocations WHERE contributor_id = @contributor_id ORDER BY revision DESC LIMIT 1;

-- name: RecordTrainingRevocation :exec
INSERT INTO privacy_training_revocations(contributor_id, revision, revoked_at)
VALUES (@contributor_id, @revision, @revoked_at)
ON CONFLICT(contributor_id, revision) DO UPDATE SET
revoked_at = LEAST(privacy_training_revocations.revoked_at, EXCLUDED.revoked_at);

-- name: TombstoneOwnerTrainingBatch :execrows
WITH due AS (
 SELECT d.image_id, d.consent_revision, r.revoked_at
 FROM privacy_training_derivatives d JOIN LATERAL (
 SELECT v.revoked_at FROM privacy_training_revocations v WHERE v.contributor_id = d.contributor_id AND v.revision >= d.consent_revision ORDER BY v.revision LIMIT 1
 ) r ON true
 WHERE d.contributor_id = @contributor_id AND d.state = 'ACTIVE' 
 ORDER BY d.image_id,d.consent_revision LIMIT 32 FOR UPDATE OF d SKIP LOCKED
)
UPDATE privacy_training_derivatives target SET state = 'REVOKED', revoked_at = due.revoked_at
FROM due WHERE target.image_id = due.image_id AND target.consent_revision = due.consent_revision AND target.state = 'ACTIVE';

-- name: TombstoneTrainingBatch :execrows
WITH due AS (
 SELECT d.image_id, d.consent_revision, r.revoked_at
 FROM privacy_training_derivatives d JOIN LATERAL (
 SELECT v.revoked_at FROM privacy_training_revocations v WHERE v.contributor_id = d.contributor_id AND v.revision >= d.consent_revision ORDER BY v.revision LIMIT 1
 ) r ON true
 WHERE d.state = 'ACTIVE' 
 ORDER BY r.revoked_at,d.image_id,d.consent_revision LIMIT 32 FOR UPDATE OF d SKIP LOCKED
)
UPDATE privacy_training_derivatives target SET state = 'REVOKED', revoked_at = due.revoked_at
FROM due WHERE target.image_id = due.image_id AND target.consent_revision = due.consent_revision AND target.state = 'ACTIVE';
