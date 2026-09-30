-- 000026: moderation COMMENT target (P14-T05A). Widens the case
-- target allowlist with COMMENT for station/fuel feedback reports
-- (additive: every previously accepted value stays accepted; no row
-- is rewritten).
ALTER TABLE moderation_cases DROP CONSTRAINT moderation_cases_target_check;
ALTER TABLE moderation_cases ADD CONSTRAINT moderation_cases_target_check CHECK (target_type IN
    ('OBSERVATION', 'DISPUTE', 'CONTRIBUTOR', 'EVIDENCE', 'COMMENT'));
