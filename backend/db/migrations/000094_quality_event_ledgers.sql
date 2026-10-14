-- FQ-04b enforcement/sanction ledgers and edition period controls.
-- Append-only; corrections supersede revisions and never rewrite rows.
-- Procedural scope/status stay source-verbatim here: FQ-06/FQ-07 own the
-- classification vocabulary. Readers only see published revisions through
-- the current pointer, scoped per source/stream.
CREATE TABLE quality_enforcement_events(
 id UUID PRIMARY KEY,
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE CASCADE,
 native_key TEXT NOT NULL CHECK(length(native_key) BETWEEN 1 AND 256),
 scope TEXT NOT NULL CHECK(length(scope) BETWEEN 1 AND 64),
 status TEXT NOT NULL CHECK(length(status) BETWEEN 1 AND 64),
 subject_cnpj TEXT NOT NULL CHECK(length(subject_cnpj) = 14),
 occurred_on DATE,
 as_of DATE NOT NULL,
 evidence_ref TEXT NOT NULL CHECK(length(evidence_ref) BETWEEN 1 AND 512),
 UNIQUE(revision_id, native_key)
);
CREATE INDEX quality_enforcement_revision_subject_idx
 ON quality_enforcement_events(revision_id, subject_cnpj);
CREATE TABLE quality_sanction_events(
 id UUID PRIMARY KEY,
 revision_id UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE CASCADE,
 native_key TEXT NOT NULL CHECK(length(native_key) BETWEEN 1 AND 256),
 proceeding_ref TEXT NOT NULL CHECK(length(proceeding_ref) BETWEEN 1 AND 256),
 party_cnpj TEXT NOT NULL CHECK(length(party_cnpj) = 14),
 amount_minor BIGINT CHECK(amount_minor >= 0),
 status TEXT NOT NULL CHECK(length(status) BETWEEN 1 AND 64),
 decided_on DATE,
 as_of DATE NOT NULL,
 evidence_ref TEXT NOT NULL CHECK(length(evidence_ref) BETWEEN 1 AND 512),
 UNIQUE(revision_id, native_key)
);
CREATE INDEX quality_sanction_revision_party_idx
 ON quality_sanction_events(revision_id, party_cnpj);
-- Claimed coverage of one edition. Missing or duplicated period labels
-- stay visible here for FQ-05 discovery; a label never proves coverage.
CREATE TABLE quality_periods(
 edition_id UUID NOT NULL REFERENCES quality_editions(id) ON DELETE CASCADE,
 period_label TEXT NOT NULL CHECK(length(period_label) BETWEEN 1 AND 32),
 period_start DATE NOT NULL,
 period_end DATE NOT NULL,
 CONSTRAINT quality_periods_order_check CHECK(period_start <= period_end),
 PRIMARY KEY(edition_id, period_label)
);
