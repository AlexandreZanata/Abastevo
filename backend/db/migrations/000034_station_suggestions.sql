-- 000034: private station suggestions (P27-T01). Append-only: one row
-- per owner idempotency key; concurrent same-station proposals converge
-- later without sharing private state. Suggestions stay private to the
-- submitter and authorized reviewers until a T02 decision; knowing a
-- public CNPJ grants nothing. No price, trust or ownership output here.
CREATE TABLE station_suggestions (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL,
    client_submission_id TEXT NOT NULL,
    proposal JSONB NOT NULL,
    evidence_ref TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    CONSTRAINT station_suggestions_state_check
        CHECK (state IN ('pending', 'cancelled', 'approved', 'rejected'))
);
CREATE UNIQUE INDEX station_suggestions_idempotency_unique
    ON station_suggestions (account_id, client_submission_id);
CREATE INDEX station_suggestions_owner_idx
    ON station_suggestions (account_id, created_at DESC);
