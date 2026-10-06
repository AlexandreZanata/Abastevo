-- 000035: suggestion review decisions (P27-T02). Append-only: one
-- audited decision row per review action; the suggestion row carries
-- only its current state. Only pending suggestions transition, guarded
-- in SQL as well as the service, so concurrent reviewers converge
-- visibly instead of overwriting. Linked stations reference canonical
-- identities; no ownership, price or trust output here.
CREATE TABLE suggestion_decisions (
    id UUID PRIMARY KEY,
    suggestion_id UUID NOT NULL REFERENCES station_suggestions (id) ON DELETE RESTRICT,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    reviewer TEXT NOT NULL DEFAULT '',
    station_id UUID REFERENCES directory_stations (id) ON DELETE RESTRICT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT suggestion_decisions_decision_check
        CHECK (decision IN ('approved', 'rejected'))
);
CREATE INDEX suggestion_decisions_suggestion_idx
    ON suggestion_decisions (suggestion_id);
