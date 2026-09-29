-- 000018: privacy request ledger (P07-T03). Durable contributor
-- export and erasure intents (BUC-007, B-BR-011/016) with bounded,
-- expiring outcomes. Retries converge on the owner natural key;
-- divergent payloads surface through the conflicting row instead of
-- forking history. READY archives are bounded bytes with a recorded
-- hash and a 24 h download window derived at read time; expiry purges
-- land with the retention scheduler (P07-T05).
CREATE TABLE privacy_requests (
    id UUID PRIMARY KEY,
    contributor_id TEXT NOT NULL,
    contributor_ref TEXT NOT NULL,
    client_submission_id TEXT NOT NULL,
    type TEXT NOT NULL CONSTRAINT privacy_requests_type_check CHECK (type IN
        ('EXPORT', 'DELETION')),
    status TEXT NOT NULL DEFAULT 'REQUESTED' CONSTRAINT privacy_requests_status_check CHECK (status IN
        ('REQUESTED', 'READY', 'FAILED')),
    archive BYTEA,
    archive_sha256 TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    policy_version TEXT NOT NULL
);
-- One pending intent per owner operation: mobile retries converge,
-- and a new client submission ID starts a fresh attempt after failure.
CREATE UNIQUE INDEX privacy_requests_natural_unique
    ON privacy_requests (contributor_id, client_submission_id);
CREATE INDEX privacy_requests_owner_idx
    ON privacy_requests (contributor_id, requested_at DESC);
CREATE INDEX privacy_requests_expiry_idx
    ON privacy_requests (expires_at)
    WHERE status = 'READY';
