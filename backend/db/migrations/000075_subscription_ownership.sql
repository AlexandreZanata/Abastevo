-- PRM01-T02: private verified purchases, owner commands and generation fences.
-- IDs 000061..000074 are reserved by the independently progressing dev work.
-- No paid/provider configuration is enabled by this schema.
CREATE TABLE subscription_verification_slots (
    token_digest TEXT PRIMARY KEY CHECK (token_digest ~ '^[0-9a-f]{64}$'),
    generation BIGINT NOT NULL CHECK (generation > 0),
    account_id UUID NOT NULL REFERENCES accounts(id),
    operation_id UUID NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL
);
CREATE TABLE subscription_operations (
    account_id UUID NOT NULL REFERENCES accounts(id),
    operation_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('verify','restore','refresh')),
    token_digest TEXT NOT NULL REFERENCES subscription_verification_slots(token_digest),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    token_cipher BYTEA,
    generation BIGINT NOT NULL CHECK (generation > 0),
    attempts INTEGER NOT NULL DEFAULT 1 CHECK (attempts BETWEEN 1 AND 5),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','denied')),
    lease_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (account_id,operation_id)
);
CREATE INDEX subscription_operations_admission_idx ON subscription_operations(account_id,created_at);
CREATE INDEX subscription_operations_pending_idx ON subscription_operations(account_id,lease_until) WHERE status='pending';
CREATE TABLE subscription_purchases (
    token_digest TEXT PRIMARY KEY CHECK (token_digest ~ '^[0-9a-f]{64}$'),
    account_id UUID NOT NULL REFERENCES accounts(id),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    package_name TEXT NOT NULL CHECK (length(package_name) BETWEEN 1 AND 150),
    product_id TEXT NOT NULL CHECK (length(product_id) BETWEEN 1 AND 150),
    base_plan_id TEXT NOT NULL CHECK (length(base_plan_id) BETWEEN 1 AND 150),
    state TEXT NOT NULL CHECK (state IN ('PENDING','ACTIVE','CANCELED','GRACE','HOLD','PAUSED','EXPIRED','REVOKED','UNKNOWN')),
    valid_until TIMESTAMPTZ,
    verified_at TIMESTAMPTZ NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    retired BOOLEAN NOT NULL DEFAULT false,
    refresh_required BOOLEAN NOT NULL DEFAULT false,
    acknowledged BOOLEAN NOT NULL DEFAULT false,
    token_cipher BYTEA,
    linked_digest TEXT CHECK (linked_digest IS NULL OR linked_digest ~ '^[0-9a-f]{64}$')
);
CREATE INDEX subscription_purchases_owner_idx ON subscription_purchases(account_id,retired,valid_until DESC,verified_at DESC);
CREATE TABLE subscription_retired_tokens (
    token_digest TEXT PRIMARY KEY CHECK (token_digest ~ '^[0-9a-f]{64}$'),
    account_id UUID NOT NULL REFERENCES accounts(id),
    retired_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
-- Account deletion must revoke/purge provider material in the same transaction,
-- even for accounts with no contributor inventory or a stopped worker.
CREATE FUNCTION subscription_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO subscription_retired_tokens(token_digest,account_id)
    SELECT token_digest,account_id FROM subscription_purchases WHERE account_id=NEW.id
    ON CONFLICT (token_digest) DO NOTHING;
    UPDATE subscription_purchases SET token_cipher=NULL,retired=true,state='REVOKED',
        refresh_required=true,revision=revision+1 WHERE account_id=NEW.id;
    UPDATE subscription_operations SET token_cipher=NULL,status='denied',completed_at=clock_timestamp()
        WHERE account_id=NEW.id;
    UPDATE subscription_verification_slots SET generation=generation+1,lease_until=clock_timestamp()
        WHERE account_id=NEW.id;
    RETURN NEW;
END $$;
CREATE TRIGGER subscription_owner_erasure AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION subscription_erase_deleted_owner();
