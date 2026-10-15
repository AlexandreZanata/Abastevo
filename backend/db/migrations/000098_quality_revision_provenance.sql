-- FQ-04C: freeze provenance with the revision, never the mutable edition index.
CREATE TABLE quality_revision_provenance (
 revision_id UUID PRIMARY KEY REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 provenance_state TEXT NOT NULL CHECK(provenance_state IN ('VERIFIED','LEGACY_UNVERIFIED')),
 input BIGINT,
 accepted BIGINT,
 duplicates BIGINT,
 quarantined BIGINT,
 manifest_hash TEXT,
 publication_key TEXT,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK((provenance_state='LEGACY_UNVERIFIED' AND input IS NULL AND accepted IS NULL AND duplicates IS NULL AND quarantined IS NULL AND manifest_hash IS NULL AND publication_key IS NULL)
  OR (provenance_state='VERIFIED' AND input IS NOT NULL AND accepted IS NOT NULL AND duplicates IS NOT NULL AND quarantined IS NOT NULL AND manifest_hash IS NOT NULL
   AND input>=0 AND accepted>=0 AND duplicates>=0 AND quarantined>=0
   AND input=accepted+duplicates+quarantined AND manifest_hash ~ '^[0-9a-f]{64}$' AND publication_key IS NOT NULL AND publication_key ~ '^[0-9a-f]{64}$'))
);
CREATE UNIQUE INDEX quality_revision_publication_key_unique ON quality_revision_provenance(publication_key) WHERE publication_key IS NOT NULL;
CREATE TABLE quality_revision_resources (
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 url TEXT NOT NULL CHECK(length(url) BETWEEN 1 AND 1024),
 sha256 TEXT NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 bytes BIGINT NOT NULL CHECK(bytes>0),
 PRIMARY KEY(revision_id,sha256,url)
);
CREATE TABLE quality_revision_periods (
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 period_label TEXT NOT NULL CHECK(length(period_label) BETWEEN 1 AND 32),
 period_start DATE NOT NULL,
 period_end DATE NOT NULL,
 CHECK(period_start<=period_end),
 PRIMARY KEY(revision_id,period_label)
);
-- The legacy edition index cannot recover earlier exact input provenance.
-- No invented counts/hashes are copied. Verified reimport appends a new revision.
INSERT INTO quality_revision_provenance(revision_id,provenance_state)
SELECT id,'LEGACY_UNVERIFIED' FROM quality_revisions;
