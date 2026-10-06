-- 000037: representation claims and one-use declarations (P30-T03).
-- Append-only: account-bound private requests with server-bound exact
-- authorization content. One active declaration per claim (reissue
-- versions and supersedes); consumed/expired declarations never
-- reactivate. No grant, proof-byte or personal-document storage here
-- (proof intake is P30-T04); no ownership output anywhere.
CREATE TABLE profile_claims (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    operator_cnpj TEXT NOT NULL DEFAULT '',
    operator_source TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '',
    policy_version TEXT NOT NULL DEFAULT 'profile-v1',
    state TEXT NOT NULL DEFAULT 'draft',
    client_key TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT profile_claims_role_check
        CHECK (role IN ('administrator', 'manager')),
    CONSTRAINT profile_claims_state_check
        CHECK (state IN ('draft', 'awaiting_proof', 'checking', 'needs_information', 'in_review', 'approved', 'denied', 'cancelled', 'expired'))
);
CREATE UNIQUE INDEX profile_claims_idempotency_unique
    ON profile_claims (account_id, client_key) WHERE client_key <> '';
CREATE INDEX profile_claims_owner_idx
    ON profile_claims (account_id, created_at DESC);
CREATE INDEX profile_claims_station_idx
    ON profile_claims (station_id) WHERE state NOT IN ('denied', 'cancelled', 'expired');
CREATE TABLE claim_declarations (
    id UUID PRIMARY KEY,
    claim_id UUID NOT NULL REFERENCES profile_claims (id) ON DELETE RESTRICT,
    version INTEGER NOT NULL DEFAULT 1,
    nonce_digest TEXT NOT NULL,
    expected_digest TEXT NOT NULL,
    declaration TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'active',
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT claim_declarations_state_check
        CHECK (state IN ('active', 'superseded', 'consumed', 'expired'))
);
CREATE UNIQUE INDEX claim_declarations_nonce_unique
    ON claim_declarations (nonce_digest);
