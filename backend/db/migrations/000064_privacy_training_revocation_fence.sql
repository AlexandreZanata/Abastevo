-- RVA04: durable monotonic restore/selection fence. No pixels or GPS.
CREATE TABLE privacy_training_revocations (
 contributor_id TEXT NOT NULL,
 revision BIGINT NOT NULL CHECK(revision > 0),
 revoked_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(contributor_id, revision)
);
INSERT INTO privacy_training_revocations(contributor_id, revision, revoked_at)
SELECT contributor_id, revision, COALESCE(revoked_at, decided_at)
FROM privacy_training_consents WHERE state = 'REVOKED';
CREATE INDEX privacy_training_derivatives_revocation_idx
ON privacy_training_derivatives(contributor_id, consent_revision, image_id)
WHERE state = 'ACTIVE';
