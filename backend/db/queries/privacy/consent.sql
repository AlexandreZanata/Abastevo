-- Owned by privacy (CS09B). Training-consent singleton rows: one per
-- contributor, moved only by the revision-guarded upsert below.
-- Greater revisions win, equal identical retries return no row (the
-- caller converges them against the stored row), smaller revisions
-- return no row (the caller maps them stale). No DELETE path and no
-- touch of sibling tables: revocation stops selection here while
-- physical removal stays with the deletion/purge paths (CS10).

-- name: GetTrainingConsent :one
SELECT contributor_id, contributor_ref, purpose, terms_version,
    terms_digest, state, revision, granted_at, revoked_at, decided_at
FROM privacy_training_consents
WHERE contributor_id = @contributor_id;

-- name: UpsertTrainingConsent :one
INSERT INTO privacy_training_consents
    (contributor_id, contributor_ref, purpose, terms_version,
     terms_digest, state, revision, granted_at, revoked_at, decided_at)
VALUES (@contributor_id, @contributor_ref, @purpose, @terms_version,
    @terms_digest, @state, @revision, @granted_at, @revoked_at,
    @decided_at)
ON CONFLICT (contributor_id) DO UPDATE SET
    contributor_ref = EXCLUDED.contributor_ref, purpose = EXCLUDED.purpose,
    terms_version = EXCLUDED.terms_version, terms_digest = EXCLUDED.terms_digest,
    state = EXCLUDED.state, revision = EXCLUDED.revision,
    granted_at = EXCLUDED.granted_at, revoked_at = EXCLUDED.revoked_at,
    decided_at = EXCLUDED.decided_at
WHERE EXCLUDED.revision > privacy_training_consents.revision
RETURNING contributor_id, contributor_ref, purpose, terms_version,
    terms_digest, state, revision, granted_at, revoked_at, decided_at;
