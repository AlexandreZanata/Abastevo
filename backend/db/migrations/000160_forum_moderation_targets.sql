-- Migration 000160_forum_moderation_targets.sql
-- Add FORUM_POST and FORUM_COMMENT targets to moderation_cases and create forum_reports table.
-- Reconciles B-BR-FOR-08, B-BR-FOR-10, B-BR-FOR-11, BUC-FOR-06.

-- 1. Update moderation_cases target allowlist
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
    'FORUM_COMMENT'
));

-- 2. Create forum_reports table for unique report receipts and abuse triage
CREATE TABLE forum_reports (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('FORUM_POST', 'FORUM_COMMENT')),
    target_id UUID NOT NULL,
    reason TEXT NOT NULL CHECK (reason IN ('SPAM', 'HARASSMENT', 'DEFAMATION', 'OFF_TOPIC', 'ILLEGAL', 'OTHER')),
    detail TEXT NOT NULL DEFAULT '' CHECK (octet_length(detail) <= 500),
    status TEXT NOT NULL DEFAULT 'RECEIVED' CHECK (status IN ('RECEIVED', 'RESOLVED', 'DISMISSED')),
    moderation_case_id UUID REFERENCES moderation_cases(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT forum_reports_account_target_uniq UNIQUE (account_id, target_type, target_id)
);

CREATE INDEX forum_reports_target_idx ON forum_reports (target_type, target_id);
CREATE INDEX forum_reports_account_created_idx ON forum_reports (account_id, created_at);
