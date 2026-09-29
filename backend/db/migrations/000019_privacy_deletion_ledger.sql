-- 000019: deletion replay ledger (P07-T04). Minimal bounded rows
-- retained to reapply removals after a restore (BUC-007, B-BR-016,
-- ADR-008): owner identifiers plus scope, one row per contributor and
-- scope. Payload never lands here; the ledger itself ages out with the
-- backup horizon under the retention scheduler (P07-T05).
CREATE TABLE privacy_deletion_ledger (
    id UUID PRIMARY KEY,
    contributor_id TEXT NOT NULL,
    contributor_ref TEXT NOT NULL,
    scope TEXT NOT NULL CONSTRAINT privacy_deletion_ledger_scope_check CHECK (scope IN
        ('IDENTITY', 'COMMUNITY', 'EVIDENCE', 'TRUST', 'EXPORTS')),
    reason TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    replayed_at TIMESTAMPTZ,
    policy_version TEXT NOT NULL
);
CREATE UNIQUE INDEX privacy_deletion_ledger_scope_unique
    ON privacy_deletion_ledger (contributor_id, scope);
CREATE INDEX privacy_deletion_ledger_contributor_idx
    ON privacy_deletion_ledger (contributor_id);
