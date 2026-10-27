-- 000140: administrative sessions, login-attempt limits, idempotency and the
-- append-only audit ledger (ADR-023, B-BR-ADM-01..12). The raw admin key and
-- raw session/CSRF tokens never persist: sessions keep a lookup digest plus a
-- salted hash, and audit rows keep only the hashed client address.
CREATE TABLE admin_sessions (
    id UUID PRIMARY KEY,
    token_lookup TEXT NOT NULL UNIQUE CHECK (token_lookup ~ '^[0-9a-f]{64}$'),
    token_salt TEXT NOT NULL CHECK (octet_length(token_salt) BETWEEN 16 AND 128),
    token_hash TEXT NOT NULL CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    csrf_salt TEXT NOT NULL CHECK (octet_length(csrf_salt) BETWEEN 16 AND 128),
    csrf_hash TEXT NOT NULL CHECK (csrf_hash ~ '^[0-9a-f]{64}$'),
    key_fingerprint TEXT NOT NULL CHECK (key_fingerprint ~ '^[0-9a-f]{12}$'),
    created_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT admin_sessions_period CHECK (expires_at > created_at),
    CONSTRAINT admin_sessions_seen CHECK (last_seen_at >= created_at AND last_seen_at <= expires_at)
);
CREATE INDEX admin_sessions_expiry_idx ON admin_sessions(expires_at)
    WHERE revoked_at IS NULL;

-- One row per fixed window: IP rows use 15-minute windows, the singleton
-- global row uses one-hour windows. Counters are incremented while the row
-- is locked, so concurrent failures never lose updates.
CREATE TABLE admin_login_attempts (
    scope TEXT NOT NULL CHECK (scope IN ('ip', 'global')),
    subject TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    failures INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    blocked_until TIMESTAMPTZ,
    PRIMARY KEY (scope, subject, window_start),
    CONSTRAINT admin_login_attempts_subject CHECK (
        (scope = 'global' AND subject = 'global') OR
        (scope = 'ip' AND subject ~ '^[0-9a-f]{64}$')
    )
);

-- One row per administrative mutation and client key. Reservation, business
-- write, audit and stored outcome share one transaction; only the body hash
-- is stored, never the body.
CREATE TABLE admin_idempotency (
    session_id UUID NOT NULL REFERENCES admin_sessions(id),
    key UUID NOT NULL,
    action TEXT NOT NULL,
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    response_code INTEGER,
    response_body BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (session_id, key)
);

CREATE TABLE admin_audit_events (
    id BIGSERIAL PRIMARY KEY,
    at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    session_id UUID REFERENCES admin_sessions(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    before JSONB,
    after JSONB,
    request_id TEXT NOT NULL DEFAULT '',
    client_ip_hash TEXT NOT NULL DEFAULT ''
);
CREATE INDEX admin_audit_events_at_idx ON admin_audit_events(at DESC, id DESC);
CREATE INDEX admin_audit_events_action_idx ON admin_audit_events(action, at DESC);

CREATE FUNCTION admin_audit_events_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'admin_audit_events is append-only';
END $$;
CREATE TRIGGER admin_audit_events_no_update BEFORE UPDATE ON admin_audit_events
FOR EACH ROW EXECUTE FUNCTION admin_audit_events_append_only();
CREATE TRIGGER admin_audit_events_no_delete BEFORE DELETE ON admin_audit_events
FOR EACH ROW EXECUTE FUNCTION admin_audit_events_append_only();
