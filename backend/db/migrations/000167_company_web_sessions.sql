-- EMP-PLAT-007. Company account sessions are separate from admin credentials.
-- Composite ownership prevents a family's cookie being assigned to another account.
CREATE UNIQUE INDEX account_session_families_company_owner_idx ON account_session_families(account_id,id);
CREATE TABLE company_web_sessions(
    id UUID PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'::uuid),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    family_id UUID NOT NULL,
    token_lookup TEXT NOT NULL UNIQUE CHECK(token_lookup ~ '^[0-9a-f]{64}$'),
    token_salt TEXT NOT NULL CHECK(token_salt ~ '^[0-9a-f]{32}$'),
    token_hash TEXT NOT NULL CHECK(token_hash ~ '^[0-9a-f]{64}$'),
    csrf_salt TEXT NOT NULL CHECK(csrf_salt ~ '^[0-9a-f]{32}$'),
    csrf_hash TEXT NOT NULL CHECK(csrf_hash ~ '^[0-9a-f]{64}$'),
    authenticated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY(account_id,family_id) REFERENCES account_session_families(account_id,id) ON DELETE CASCADE,
    CHECK(authenticated_at <= created_at AND authenticated_at > created_at - interval '15 minutes'),
    CHECK(last_seen_at >= created_at AND last_seen_at < expires_at),
    CHECK(expires_at > created_at AND expires_at <= created_at + interval '8 hours')
);
CREATE INDEX company_web_sessions_account_idx ON company_web_sessions(account_id);
CREATE INDEX company_web_sessions_cleanup_idx ON company_web_sessions(expires_at,last_seen_at);
