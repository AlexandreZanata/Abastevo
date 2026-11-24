-- Expand-only: the previous image continues to use the alias and username hash.
ALTER TABLE accounts ADD COLUMN display_name TEXT
    CONSTRAINT accounts_display_name_length_check CHECK (char_length(display_name) BETWEEN 2 AND 40);

ALTER TABLE account_addresses ADD COLUMN email_hint TEXT;

CREATE TABLE account_recovery_audit (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    action TEXT NOT NULL CONSTRAINT account_recovery_audit_action_check
        CHECK (action IN ('recovery', 'email_add', 'email_change')),
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX account_recovery_audit_retention_idx
    ON account_recovery_audit (occurred_at, id);

-- The older image also clears the profile name on deletion during rollback.
CREATE FUNCTION account_clear_deleted_display_name() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.display_name = NULL;
    RETURN NEW;
END $$;

CREATE TRIGGER accounts_clear_deleted_display_name
    BEFORE UPDATE OF status ON accounts
    FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION account_clear_deleted_display_name();
