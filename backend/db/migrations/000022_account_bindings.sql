-- 000022: account contributor bindings + link audit (P13-T04B, B-BR-A04).
-- Identity-adjacent but separately owned: one row per account and
-- anonymous contributor, binding the device key that may write under
-- the account. A contributor binds at most one account: the UNIQUE
-- contributor guard plus the application stolen-ID refusal keep
-- recovery from transferring another contributor's reputation.
-- Every bind/unbind appends an audit row (180-day inventory horizon,
-- enforced by the retention scheduler). Append-only; later phases wire
-- transport and cross-module erasure, never rewrite.
CREATE TABLE account_contributor_bindings (
    account_id UUID NOT NULL REFERENCES accounts (id),
    contributor_id TEXT NOT NULL CONSTRAINT account_contributor_bindings_contributor_check CHECK (contributor_id <> ''),
    key_fingerprint TEXT NOT NULL CONSTRAINT account_contributor_bindings_fingerprint_check CHECK (key_fingerprint <> ''),
    bound_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_contributor_bindings_pk PRIMARY KEY (account_id, contributor_id),
    CONSTRAINT account_contributor_bindings_owner_unique UNIQUE (contributor_id)
);
CREATE INDEX account_contributor_bindings_account_idx
    ON account_contributor_bindings (account_id);
CREATE TABLE account_binding_audit (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    contributor_id TEXT NOT NULL,
    action TEXT NOT NULL CONSTRAINT account_binding_audit_action_check CHECK (action IN
        ('bind', 'unbind')),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX account_binding_audit_account_idx
    ON account_binding_audit (account_id, occurred_at DESC);
