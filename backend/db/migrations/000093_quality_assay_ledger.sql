-- FQ-04a assay ledger. Append-only; corrections supersede revisions and
-- never rewrite rows. Assay/enforcement/sanction streams share editions,
-- manifests and revision controls; enforcement/sanction fact tables arrive
-- with FQ-04b. Readers only see published revisions through the current
-- pointer; staging rows stay invisible until the atomic switch.
CREATE TABLE quality_editions(
 id UUID PRIMARY KEY,
 source TEXT NOT NULL CHECK(source IN ('pmqc', 'enforcement', 'sanctions')),
 stream TEXT NOT NULL CHECK(length(stream) BETWEEN 1 AND 64),
 edition TEXT NOT NULL CHECK(length(edition) BETWEEN 1 AND 32),
 predecessor TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(source, stream, edition)
);
CREATE TABLE quality_resources(
 edition_id UUID NOT NULL REFERENCES quality_editions(id) ON DELETE CASCADE,
 url TEXT NOT NULL CHECK(length(url) BETWEEN 1 AND 1024),
 sha256 TEXT NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 bytes BIGINT NOT NULL CHECK(bytes > 0),
 PRIMARY KEY(edition_id, sha256)
);
CREATE TABLE quality_manifests(
 edition_id UUID PRIMARY KEY REFERENCES quality_editions(id) ON DELETE CASCADE,
 input BIGINT NOT NULL CHECK(input >= 0),
 accepted BIGINT NOT NULL CHECK(accepted >= 0),
 duplicates BIGINT NOT NULL CHECK(duplicates >= 0),
 quarantined BIGINT NOT NULL CHECK(quarantined >= 0),
 manifest_hash TEXT NOT NULL CHECK(length(manifest_hash) = 64),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CONSTRAINT quality_manifests_balance_check
  CHECK(input = accepted + duplicates + quarantined)
);
CREATE TABLE quality_revisions(
 id UUID PRIMARY KEY,
 edition_id UUID NOT NULL REFERENCES quality_editions(id) ON DELETE RESTRICT,
 status TEXT NOT NULL DEFAULT 'staging',
 supersedes_revision_id UUID REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 published_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CONSTRAINT quality_revisions_status_check
  CHECK(status IN ('staging', 'published', 'superseded', 'withdrawn'))
);
CREATE TABLE quality_assays(
 id UUID PRIMARY KEY,
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE CASCADE,
 sample_ref TEXT NOT NULL CHECK(length(sample_ref) BETWEEN 1 AND 128),
 assay_ref TEXT NOT NULL CHECK(length(assay_ref) BETWEEN 1 AND 128),
 source_cnpj TEXT NOT NULL CHECK(length(source_cnpj) = 14),
 observed_on DATE NOT NULL,
 fuel TEXT NOT NULL CHECK(fuel IN
  ('ETHANOL', 'GASOLINE_REGULAR', 'GASOLINE_ADDITIVED',
   'GASOLINE_PREMIUM_GRADE', 'DIESEL_S500', 'DIESEL_S10', 'CNG', 'LPG_P13')),
 result_text TEXT NOT NULL CHECK(length(result_text) BETWEEN 1 AND 1024),
 unit TEXT NOT NULL CHECK(length(unit) BETWEEN 0 AND 64),
 conformity TEXT NOT NULL CHECK(conformity IN
  ('CONFORMING', 'NONCONFORMING', 'INCONCLUSIVE', 'UNKNOWN')),
 evidence_ref TEXT NOT NULL CHECK(length(evidence_ref) BETWEEN 1 AND 512),
 fingerprint TEXT NOT NULL CHECK(length(fingerprint) = 64),
 UNIQUE(revision_id, fingerprint)
);
CREATE INDEX quality_assays_revision_sample_idx
 ON quality_assays(revision_id, sample_ref, assay_ref);
CREATE TABLE quality_links(
 id UUID PRIMARY KEY,
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE CASCADE,
 record_key TEXT NOT NULL CHECK(length(record_key) BETWEEN 1 AND 256),
 station_id UUID REFERENCES directory_stations(id) ON DELETE RESTRICT,
 automatic BOOLEAN NOT NULL DEFAULT FALSE,
 reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 128),
 active BOOLEAN NOT NULL DEFAULT TRUE,
 UNIQUE(revision_id, record_key, station_id)
);
CREATE INDEX quality_links_revision_active_idx
 ON quality_links(revision_id, active) WHERE active;
CREATE TABLE quality_current_revisions(
 source TEXT NOT NULL,
 stream TEXT NOT NULL,
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 switched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(source, stream)
);
-- Exactly one published revision per edition: concurrent publishers
-- serialize on this index, and corrections supersede instead of coexisting.
CREATE UNIQUE INDEX quality_revisions_one_published_per_edition
 ON quality_revisions(edition_id) WHERE status = 'published';
