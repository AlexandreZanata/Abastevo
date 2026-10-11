-- 000057: privacy training-derivative inventory (CS10B). One row per
-- image plus consent revision holding the minimized derivative
-- reference (B-BR-CS06, AT03/AT05): owner binding, source and
-- derivative hashes, capture instant, frozen terms and the ACTIVE /
-- REVOKED lifecycle. Append-only: writes converge on the natural
-- key, revocation flips ACTIVE to REVOKED through the guarded
-- transition in derivatives.sql, and no DELETE path exists in this
-- or any query file. Revoked receipts stay for the restore filter;
-- the purge path owns physical bytes (CS10C). Absent rows simply
-- never select.
CREATE TABLE privacy_training_derivatives (
  image_id TEXT NOT NULL CHECK (char_length(image_id) BETWEEN 1 AND 128),
  consent_revision BIGINT NOT NULL CHECK (consent_revision >= 1),
  contributor_id TEXT NOT NULL,
  contributor_ref TEXT NOT NULL,
  source_sha256 TEXT NOT NULL,
  derivative_sha256 TEXT NOT NULL,
  captured_at TIMESTAMPTZ NOT NULL,
  terms_version TEXT NOT NULL,
  terms_digest TEXT NOT NULL CHECK (char_length(terms_digest) BETWEEN 1 AND 128),
  state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'REVOKED')),
  promoted_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  PRIMARY KEY (image_id, consent_revision)
);
CREATE INDEX privacy_training_derivatives_owner_idx
  ON privacy_training_derivatives (contributor_id, state);
