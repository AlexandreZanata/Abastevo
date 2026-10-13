-- Explicit OFF before any grant has no reviewed terms pair. Preserve existing
-- decisions and retain the nonempty digest requirement for every grant.
-- The owner revision and revocation fence remain unchanged.
ALTER TABLE privacy_training_consents
  DROP CONSTRAINT privacy_training_consents_terms_digest_check;
ALTER TABLE privacy_training_consents
  ADD CONSTRAINT privacy_training_consents_terms_digest_check CHECK (
    char_length(terms_digest) BETWEEN 1 AND 128
    OR (state = 'REVOKED' AND terms_version = '' AND terms_digest = '')
  );
