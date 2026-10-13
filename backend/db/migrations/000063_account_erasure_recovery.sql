-- RVA03: session-independent recovery, including binding-free accounts.
ALTER TABLE account_erasure_intents ADD COLUMN completed_at TIMESTAMPTZ;
CREATE TABLE account_erasure_requests (
    account_id UUID PRIMARY KEY REFERENCES accounts(id),
    requested_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    lease_nonce UUID,
    lease_until TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CHECK ((lease_nonce IS NULL) = (lease_until IS NULL))
);
CREATE INDEX account_erasure_requests_due_idx ON account_erasure_requests(next_attempt_at, account_id)
    WHERE completed_at IS NULL;
CREATE INDEX account_erasure_intents_pending_idx ON account_erasure_intents(account_id, contributor_id)
    WHERE completed_at IS NULL;
-- Deleted accounts predate account-wide intents. Replaying their idempotent
-- scopes is safe; do not infer cleanup from revoked credentials.
INSERT INTO account_erasure_requests(account_id, requested_at, next_attempt_at)
SELECT id, created_at, now() FROM accounts WHERE status = 'deleted';
