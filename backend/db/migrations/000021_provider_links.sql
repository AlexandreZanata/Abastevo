-- 000021: FREE provider links + OIDC nonces (P13-T03B, B-BR-A03).
-- Identity-adjacent but separately owned: one row per account and
-- provider (google/apple) binding the verified opaque subject, plus a
-- single-use nonce ledger shared across processes. Email stays
-- display/relay only; no merge key lives here. Append-only; later
-- phases add suspension/erasure enforcement, never rewrite.
CREATE TABLE account_provider_links (
    account_id UUID NOT NULL REFERENCES accounts (id),
    provider TEXT NOT NULL CONSTRAINT account_provider_links_provider_check CHECK (provider IN
        ('google', 'apple')),
    issuer TEXT NOT NULL CONSTRAINT account_provider_links_issuer_check CHECK (issuer <> ''),
    subject TEXT NOT NULL CONSTRAINT account_provider_links_subject_check CHECK (subject <> ''),
    email TEXT NOT NULL DEFAULT '',
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_provider_links_pk PRIMARY KEY (account_id, provider),
    CONSTRAINT account_provider_links_subject_unique UNIQUE (provider, subject)
);
CREATE INDEX account_provider_links_account_idx
    ON account_provider_links (account_id);
CREATE TABLE account_oidc_nonces (
    nonce TEXT PRIMARY KEY CONSTRAINT account_oidc_nonces_nonce_check CHECK (nonce <> ''),
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
