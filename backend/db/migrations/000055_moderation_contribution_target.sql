-- 000055: moderation CONTRIBUTION target (CS05A). Widens the case
-- target allowlist with CONTRIBUTION for social-abuse reports on
-- community price contributions (additive: every previously accepted
-- value stays accepted; no row is rewritten). One-open-case-per-target
-- and the open queue index apply unchanged through the existing
-- partial unique index.
ALTER TABLE moderation_cases DROP CONSTRAINT moderation_cases_target_check;
ALTER TABLE moderation_cases ADD CONSTRAINT moderation_cases_target_check CHECK (target_type IN
    ('OBSERVATION', 'DISPUTE', 'CONTRIBUTOR', 'EVIDENCE', 'COMMENT', 'CONTRIBUTION'));
