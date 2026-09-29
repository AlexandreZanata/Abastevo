-- 000010: private evidence sessions and objects (P05-T04).
-- Sessions are reservations with a guarded mutable lifecycle
-- (ISSUED→VERIFYING→READY/REJECTED, idle ISSUED→EXPIRED); every mutation
-- is a conditional UPDATE on the expected state, so concurrent workers
-- converge instead of forking. Objects are immutable verified facts:
-- one row per sanitized snapshot, pointed at only after upload success.
-- Binding to at most one observation is a single nullable column claimed
-- with set-if-unbound-or-same semantics; reuse across observations
-- conflicts instead of double-counting. No UPDATE or DELETE path exists
-- for objects anywhere in the owned queries.
CREATE TABLE evidence_sessions (
    id UUID PRIMARY KEY,
    contributor_ref TEXT NOT NULL,
    client_session_id TEXT NOT NULL,
    mime TEXT NOT NULL,
    declared_bytes BIGINT NOT NULL CHECK (declared_bytes >= 1),
    claimed_sha256 TEXT NOT NULL,
    quarantine_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'ISSUED' CONSTRAINT evidence_sessions_status_check CHECK (status IN
        ('ISSUED', 'VERIFYING', 'READY', 'REJECTED', 'EXPIRED')),
    status_reason TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    policy_version TEXT NOT NULL
);
CREATE UNIQUE INDEX evidence_sessions_natural_unique
    ON evidence_sessions (contributor_ref, client_session_id);
CREATE INDEX evidence_sessions_expiry_idx
    ON evidence_sessions (status, expires_at);
CREATE INDEX evidence_sessions_contributor_created_idx
    ON evidence_sessions (contributor_ref, created_at DESC);
CREATE TABLE evidence_objects (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES evidence_sessions (id) ON DELETE RESTRICT,
    final_key TEXT NOT NULL UNIQUE,
    source_sha256 TEXT NOT NULL,
    sanitized_sha256 TEXT NOT NULL,
    width INTEGER NOT NULL CHECK (width >= 1),
    height INTEGER NOT NULL CHECK (height >= 1),
    dhash BIGINT NOT NULL,
    bound_observation_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX evidence_objects_session_idx
    ON evidence_objects (session_id);
