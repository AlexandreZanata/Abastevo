-- 000016: moderation case queue (P07-T01). Bounded actionable review
-- items from reports and signals (BUC-006, B-BR-012). One row names one
-- target with a priority and a mandatory reason; it never copies raw
-- media, exact GPS, IP or URLs (B-BR-011). Status opens OPEN; audited
-- operator actions (P07-T02) move IN_REVIEW→RESOLVED/REJECTED through a
-- later actions table, never edits of the target fact.
CREATE TABLE moderation_cases (
    id UUID PRIMARY KEY,
    target_type TEXT NOT NULL CONSTRAINT moderation_cases_target_check CHECK (target_type IN
        ('OBSERVATION', 'DISPUTE', 'CONTRIBUTOR', 'EVIDENCE')),
    target_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CONSTRAINT moderation_cases_status_check CHECK (status IN
        ('OPEN', 'IN_REVIEW', 'RESOLVED', 'REJECTED')),
    priority TEXT NOT NULL CONSTRAINT moderation_cases_priority_check CHECK (priority IN
        ('P1', 'P2', 'P3')),
    reason TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    evidence_id UUID,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    policy_version TEXT NOT NULL
);
-- One open case per target: duplicate reports converge instead of
-- flooding the queue. Resolved history stays queryable; reopening a
-- closed target inserts a new row, never an edit.
CREATE UNIQUE INDEX moderation_cases_open_target_unique
    ON moderation_cases (target_type, target_id)
    WHERE status IN ('OPEN', 'IN_REVIEW');
-- Operator queue order: highest priority first, then oldest first with
-- a stable UUID tie-break.
CREATE INDEX moderation_cases_open_queue_idx
    ON moderation_cases (priority, opened_at ASC, id ASC)
    WHERE status IN ('OPEN', 'IN_REVIEW');
CREATE INDEX moderation_cases_target_idx
    ON moderation_cases (target_type, target_id);
