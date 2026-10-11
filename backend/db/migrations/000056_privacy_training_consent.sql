-- 000056: privacy training-consent singleton (CS09B). One row per
-- contributor holding the current training-derivative decision
-- (B-BR-CS06, P37-PC05): frozen TRAINING purpose, recorded
-- terms/digest, GRANTED/REVOKED state and the monotonic revision that
-- orders offline-queued decisions. Append-only: state moves only
-- through the revision-guarded upsert in consent.sql (greater
-- revisions win, equal identical replays, smaller is stale). No DELETE
-- path in this or any query file: revocation stops selection and
-- deletion-ledger/purge paths own physical removal (CS10). Absent
-- rows deny by default; no other table is touched.
CREATE TABLE privacy_training_consents (
  contributor_id TEXT PRIMARY KEY,
  contributor_ref TEXT NOT NULL,
  purpose TEXT NOT NULL DEFAULT 'TRAINING' CHECK (purpose = 'TRAINING'),
  terms_version TEXT NOT NULL,
  terms_digest TEXT NOT NULL CHECK (char_length(terms_digest) BETWEEN 1 AND 128),
  state TEXT NOT NULL CHECK (state IN ('GRANTED', 'REVOKED')),
  revision BIGINT NOT NULL CHECK (revision >= 1),
  granted_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  decided_at TIMESTAMPTZ NOT NULL
);
