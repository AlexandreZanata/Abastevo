-- 000143: code-frozen staff role assignments (ADR-023, B-BR-STF-01..06).
-- Roles exist only in code; the table stores assignments, never the catalog.
CREATE TABLE staff_assignments (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    role TEXT NOT NULL CHECK (role IN ('community_moderator', 'station_reviewer', 'contributor_curator', 'support_agent')),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'REVOKED')),
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ,
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 10 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    created_session_id UUID REFERENCES admin_sessions(id),
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT,
    CONSTRAINT staff_assignments_period CHECK (end_at IS NULL OR end_at > start_at),
    CONSTRAINT staff_assignments_revocation CHECK (
        (state = 'ACTIVE' AND revoked_at IS NULL AND revoke_reason IS NULL) OR
        (state = 'REVOKED' AND revoked_at IS NOT NULL AND revoke_reason IS NOT NULL)
    )
);
CREATE UNIQUE INDEX staff_assignments_active_role
    ON staff_assignments(account_id, role) WHERE state = 'ACTIVE';
CREATE INDEX staff_assignments_owner_idx ON staff_assignments(account_id, state, end_at);
CREATE INDEX staff_assignments_public_idx ON staff_assignments(state, start_at, end_at);

CREATE FUNCTION staff_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE staff_assignments
    SET state = 'REVOKED', revoked_at = clock_timestamp(), revoke_reason = 'account-erased'
    WHERE account_id = NEW.id AND state = 'ACTIVE';
    RETURN NEW;
END $$;
CREATE TRIGGER staff_owner_erasure AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION staff_erase_deleted_owner();
