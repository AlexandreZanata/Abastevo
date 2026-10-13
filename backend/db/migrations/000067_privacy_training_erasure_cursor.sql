-- RVA07: resumable owner erasure scans every receipt, including restored bytes
-- beside a prior PURGED acknowledgement. No image bytes or keys in the cursor.
CREATE TABLE privacy_training_erasure_cursors (
 contributor_id TEXT PRIMARY KEY,
 last_image_id TEXT NOT NULL CHECK(char_length(last_image_id) BETWEEN 1 AND 128),
 last_revision BIGINT NOT NULL CHECK(last_revision >= 1),
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX privacy_training_owner_keyset_idx
 ON privacy_training_derivatives(contributor_id,image_id,consent_revision);
