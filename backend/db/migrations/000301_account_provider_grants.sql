-- 000301: Sign in with Apple grant revocation (P13-T05, App Store
-- guideline 5.1.1(v)). A native sign-in that carries an authorization
-- code may keep the provider refresh token, sealed with AES-256-GCM under
-- a key that never enters the database and bound to the grant id. One
-- row per account and subject; a newer grant replaces the older one.
-- Account deletion moves the sealed grants into the revocation queue in
-- the deletion transaction; the worker revokes each at the provider and
-- deletes the row on success or after the bounded retry window. The
-- queue keeps no account, subject or email. Additive and append-only.
CREATE TABLE account_provider_grants (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    provider TEXT NOT NULL CONSTRAINT account_provider_grants_provider_check CHECK (provider = 'apple'),
    subject TEXT NOT NULL CONSTRAINT account_provider_grants_subject_check CHECK (subject <> ''),
    sealed_token BYTEA NOT NULL CONSTRAINT account_provider_grants_sealed_check
        CHECK (octet_length(sealed_token) BETWEEN 30 AND 4096),
    captured_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT account_provider_grants_subject_unique UNIQUE (account_id, provider, subject)
);
CREATE TABLE account_provider_revocations (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CONSTRAINT account_provider_revocations_provider_check CHECK (provider = 'apple'),
    sealed_token BYTEA NOT NULL CONSTRAINT account_provider_revocations_sealed_check
        CHECK (octet_length(sealed_token) BETWEEN 30 AND 4096),
    requested_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CONSTRAINT account_provider_revocations_attempts_check CHECK (attempts >= 0),
    lease_nonce UUID,
    lease_until TIMESTAMPTZ,
    CONSTRAINT account_provider_revocations_lease_check CHECK ((lease_nonce IS NULL) = (lease_until IS NULL))
);
CREATE INDEX account_provider_revocations_due_idx
    ON account_provider_revocations (next_attempt_at, id);
-- Deletion queues sealed grants in the deleting transaction itself, the
-- same way subscription_erase_deleted_owner purges purchase tokens, so no
-- deletion path (or older application code) can skip the revocation.
CREATE FUNCTION account_queue_provider_revocations() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO account_provider_revocations (id, provider, sealed_token, requested_at, next_attempt_at)
    SELECT g.id, g.provider, g.sealed_token, clock_timestamp(), clock_timestamp()
    FROM account_provider_grants g WHERE g.account_id = NEW.id
    ON CONFLICT (id) DO NOTHING;
    DELETE FROM account_provider_grants WHERE account_id = NEW.id;
    RETURN NEW;
END $$;
CREATE TRIGGER account_provider_grants_erasure AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION account_queue_provider_revocations();
