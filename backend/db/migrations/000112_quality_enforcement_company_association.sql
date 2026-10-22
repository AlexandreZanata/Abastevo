-- Enforcement company observations never inherit dated PMQC link/site
-- semantics nor sanction event bindings. Existing enforcement source rows,
-- publication keys and pointers survive byte-for-byte; no association
-- backfill. Decisions stay company-only under the current-company policy;
-- a later read stage must fence live bindings in its own transaction.
CREATE TABLE quality_enforcement_company_association_revisions (
 revision_id UUID PRIMARY KEY REFERENCES quality_revisions(id) ON DELETE CASCADE,
 policy_version TEXT NOT NULL CHECK(policy_version='current-company-v1'),
 association_scope TEXT NOT NULL CHECK(association_scope='CURRENT_COMPANY_ONLY'),
 decision_day DATE NOT NULL CHECK(isfinite(decision_day) AND decision_day BETWEEN DATE '0001-01-01' AND DATE '9999-12-31'),
 historical_business_window_known BOOLEAN NOT NULL DEFAULT FALSE CHECK(NOT historical_business_window_known),
 observation_count INTEGER NOT NULL CHECK(observation_count BETWEEN 0 AND 500000)
);

CREATE TABLE quality_enforcement_company_associations (
 revision_id UUID NOT NULL REFERENCES quality_enforcement_company_association_revisions(revision_id) ON DELETE CASCADE,
 record_key TEXT NOT NULL,
 resolution_kind TEXT NOT NULL,
 reason TEXT NOT NULL,
 station_id UUID REFERENCES directory_stations(id) ON DELETE RESTRICT,
 PRIMARY KEY(revision_id,record_key),
 FOREIGN KEY(revision_id,record_key) REFERENCES quality_enforcement_source_records(revision_id,native_key) ON DELETE CASCADE,
 CONSTRAINT quality_enforcement_company_resolution CHECK(
  (resolution_kind='bound' AND station_id IS NOT NULL AND reason='exact current cnpj; historical business window unknown') OR
  (resolution_kind='unmatched' AND station_id IS NULL AND reason='unmatched_cnpj') OR
  (resolution_kind='ambiguous' AND station_id IS NULL AND reason='ambiguous_binding') OR
  (resolution_kind='wrong_branch' AND station_id IS NULL AND reason='wrong_branch') OR
  (resolution_kind='succession_review' AND station_id IS NULL AND reason='succession_review')
 )
);
CREATE INDEX quality_enforcement_company_station_revision_key_idx
 ON quality_enforcement_company_associations(station_id,revision_id,record_key) WHERE station_id IS NOT NULL;
