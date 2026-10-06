-- Key accounts: passwordless username + server-issued account key (no
-- email, no provider, no device key). Only the salted verifier and the
-- unsalted lookup persist; the key itself never reaches storage.
-- Lookup finds the candidate row; the salted hash still authenticates.

CREATE TABLE account_key_credentials (
    account_id    uuid PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    username_hash text NOT NULL UNIQUE,
    key_lookup    text NOT NULL UNIQUE,
    key_salt      text NOT NULL,
    key_hash      text NOT NULL,
    created_at    bigint NOT NULL
);
