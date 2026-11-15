-- 000239: community user safety (B-BR-SAFE-01…09, Google Play UGC policy).
-- Private per-account blocks and user reports. A user is named only through
-- public content; account identifiers never leave the server. Account
-- erasure deletes blocks in both directions and scrubs report detail
-- through the bounded erasure batch. Append-only.

-- 1. User reports open moderation cases on the reported account.
ALTER TABLE moderation_cases DROP CONSTRAINT moderation_cases_target_check;
ALTER TABLE moderation_cases ADD CONSTRAINT moderation_cases_target_check CHECK (target_type IN (
    'OBSERVATION',
    'DISPUTE',
    'CONTRIBUTOR',
    'EVIDENCE',
    'COMMENT',
    'CONTRIBUTION',
    'STATION_CLAIM',
    'QUALITY_REPORT',
    'FORUM_POST',
    'FORUM_COMMENT',
    'COMMUNITY_ACCOUNT'
));

-- 2. Active blocks: one row per blocker/blocked pair, deleted on unblock.
CREATE TABLE community_user_blocks (
    id UUID PRIMARY KEY,
    blocker_account_id UUID NOT NULL REFERENCES accounts (id),
    blocked_account_id UUID NOT NULL REFERENCES accounts (id),
    label TEXT NOT NULL DEFAULT '' CHECK (char_length(label) <= 64),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('FORUM_POST', 'FORUM_COMMENT', 'STATION_COMMENT')),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT community_user_blocks_pair_uniq UNIQUE (blocker_account_id, blocked_account_id),
    CONSTRAINT community_user_blocks_not_self CHECK (blocker_account_id <> blocked_account_id)
);
CREATE INDEX community_user_blocks_blocker_idx
    ON community_user_blocks (blocker_account_id, created_at DESC, id DESC);
CREATE INDEX community_user_blocks_blocked_idx ON community_user_blocks (blocked_account_id);

-- 3. User reports: one per reporter/reported pair; free text stays here,
-- never in moderation_cases, so erasure can scrub it.
CREATE TABLE community_user_reports (
    id UUID PRIMARY KEY,
    reporter_account_id UUID NOT NULL REFERENCES accounts (id),
    reported_account_id UUID NOT NULL REFERENCES accounts (id),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('FORUM_POST', 'FORUM_COMMENT', 'STATION_COMMENT')),
    source_id UUID NOT NULL,
    reason TEXT NOT NULL CHECK (reason IN ('SPAM', 'HARASSMENT', 'HATE', 'IMPERSONATION', 'ILLEGAL', 'OTHER')),
    detail TEXT NOT NULL DEFAULT '' CHECK (char_length(detail) <= 200),
    moderation_case_id UUID REFERENCES moderation_cases (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT community_user_reports_pair_uniq UNIQUE (reporter_account_id, reported_account_id),
    CONSTRAINT community_user_reports_not_self CHECK (reporter_account_id <> reported_account_id)
);
CREATE INDEX community_user_reports_reporter_idx
    ON community_user_reports (reporter_account_id, created_at);
CREATE INDEX community_user_reports_reported_idx ON community_user_reports (reported_account_id);
