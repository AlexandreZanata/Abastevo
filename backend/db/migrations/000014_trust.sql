-- 000014: conservative trust ledger (P06-T03). Immutable trust
-- decisions plus a rebuildable current-tier view per contributor.
-- Decisions append only: corrections, blocks and rehabilitations arrive
-- as new rows, never edits. The current view converges on the latest
-- verdict written in the same transaction, so readers never observe a
-- decision without its projection.
CREATE TABLE trust_decisions (
    id UUID PRIMARY KEY,
    contributor_ref TEXT NOT NULL,
    tier TEXT NOT NULL CONSTRAINT trust_decisions_tier_check CHECK (tier IN
        ('NEW', 'ESTABLISHED', 'BLOCKED')),
    reason TEXT NOT NULL,
    case_refs TEXT[] NOT NULL DEFAULT '{}',
    policy_version TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX trust_decisions_contributor_idx
    ON trust_decisions (contributor_ref, occurred_at ASC, id ASC);
CREATE TABLE trust_current (
    contributor_ref TEXT PRIMARY KEY,
    tier TEXT NOT NULL CONSTRAINT trust_current_tier_check CHECK (tier IN
        ('NEW', 'ESTABLISHED', 'BLOCKED')),
    version BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
