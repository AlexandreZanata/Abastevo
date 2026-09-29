-- 000017: moderation audit ledger (P07-T02). Append-only operator
-- actions behind every case status move (BUC-006, B-BR-012): actor and
-- reason are mandatory, history is never edited, and appeals arrive as
-- new actions or new reviewed facts. The actions table itself is
-- insert-only; the single guarded status transition lives in the
-- queries file with a terminal-state guard.
CREATE TABLE moderation_actions (
    id UUID PRIMARY KEY,
    case_id UUID NOT NULL REFERENCES moderation_cases (id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL CONSTRAINT moderation_actions_action_check CHECK (action IN
        ('REVIEW', 'INVALIDATE', 'BLOCK', 'RESOLVE', 'DISMISS')),
    reason TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    policy_version TEXT NOT NULL
);
CREATE INDEX moderation_actions_case_idx
    ON moderation_actions (case_id, occurred_at ASC, id ASC);
