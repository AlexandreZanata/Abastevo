-- 000039: claim decisions and representation grants (P31-T03).
-- Append-only audited review outcomes plus narrowly scoped capability
-- grants. One active grant per account/station (partial unique index);
-- concurrent approvals converge instead of duplicating authority.
-- Grants confer representation, never ownership, tenancy or moderator
-- power. No public admin route consumes these tables directly.
CREATE TABLE claim_decisions (
    id UUID PRIMARY KEY,
    claim_id UUID NOT NULL REFERENCES profile_claims (id) ON DELETE RESTRICT,
    reviewer TEXT NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    policy_version TEXT NOT NULL,
    proof_version INTEGER NOT NULL DEFAULT 0,
    operator_cnpj TEXT NOT NULL DEFAULT '',
    scopes TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT claim_decisions_decision_check
        CHECK (decision IN ('approved', 'denied'))
);
CREATE INDEX claim_decisions_claim_idx
    ON claim_decisions (claim_id);
CREATE TABLE representation_grants (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    operator_cnpj TEXT NOT NULL,
    role TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'active',
    claim_id UUID NOT NULL REFERENCES profile_claims (id) ON DELETE RESTRICT,
    decision_id UUID NOT NULL REFERENCES claim_decisions (id) ON DELETE RESTRICT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ,
    CONSTRAINT representation_grants_role_check
        CHECK (role IN ('administrator', 'manager')),
    CONSTRAINT representation_grants_status_check
        CHECK (status IN ('active', 'suspended', 'revoked', 'expired'))
);
CREATE UNIQUE INDEX representation_grants_active_unique
    ON representation_grants (account_id, station_id) WHERE status = 'active';
