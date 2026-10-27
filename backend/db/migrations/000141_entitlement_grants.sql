-- 000141: admin-granted tiers (ADR-023, B-BR-ENT-01..12). Grants are a third
-- entitlement source beside Play and web payment grants. Capabilities are
-- derived from the code-frozen catalog; the table stores no capability array.
CREATE TABLE entitlement_grants (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    tier TEXT NOT NULL CHECK (tier IN ('PRO', 'CONTRIBUTOR', 'ENTERPRISE')),
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'REVOKED')),
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ,
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 10 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    created_session_id UUID REFERENCES admin_sessions(id),
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT,
    CONSTRAINT entitlement_grants_period CHECK (end_at IS NULL OR end_at > start_at),
    CONSTRAINT entitlement_grants_revocation CHECK (
        (state = 'ACTIVE' AND revoked_at IS NULL AND revoke_reason IS NULL) OR
        (state = 'REVOKED' AND revoked_at IS NOT NULL AND revoke_reason IS NOT NULL)
    )
);
CREATE UNIQUE INDEX entitlement_grants_active_tier
    ON entitlement_grants(account_id, tier) WHERE state = 'ACTIVE';
CREATE INDEX entitlement_grants_owner_idx
    ON entitlement_grants(account_id, state, end_at);
CREATE INDEX entitlement_grants_list_idx
    ON entitlement_grants(state, tier, created_at DESC, id DESC);

-- ENT-11: account deletion revokes every active grant in the same transaction.
CREATE FUNCTION entitlement_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE entitlement_grants
    SET state = 'REVOKED', revoked_at = clock_timestamp(), revoke_reason = 'account-erased'
    WHERE account_id = NEW.id AND state = 'ACTIVE';
    RETURN NEW;
END $$;
CREATE TRIGGER entitlement_owner_erasure AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION entitlement_erase_deleted_owner();
