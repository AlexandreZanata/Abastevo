-- 000020: FREE accounts (P13-T02B, B-BR-A01…A05). Identity-adjacent but
-- separately owned: accounts, address links, email-code verifiers and
-- rotating session families. Only salted hashes persist; codes and tokens
-- never touch storage. Append-only; later phases add columns, never rewrite.
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    alias TEXT NOT NULL,
    status TEXT NOT NULL CONSTRAINT accounts_status_check CHECK (status IN
        ('active', 'suspended', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX accounts_alias_unique ON accounts (alias);
CREATE TABLE account_addresses (
    address_hash TEXT PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX account_addresses_account_idx ON account_addresses (account_id);
CREATE TABLE account_email_codes (
    id UUID PRIMARY KEY,
    address_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    hash TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    consumed_at TIMESTAMPTZ
);
CREATE INDEX account_email_codes_address_idx
    ON account_email_codes (address_hash, issued_at DESC);
CREATE TABLE account_session_families (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    refresh_salt TEXT NOT NULL,
    refresh_hash TEXT NOT NULL,
    access_salt TEXT NOT NULL,
    access_hash TEXT NOT NULL,
    access_expires TIMESTAMPTZ NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX account_session_families_account_idx
    ON account_session_families (account_id);
