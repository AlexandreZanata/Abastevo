-- 000303: account notification inbox (B-BR-NOTIF-02, B-BR-NOTIF-03).
-- One row per (account_id, event_key): a repeated or retried producer
-- never notifies twice. Producers insert in the transaction of the event
-- they describe. Rows expire 30 days after creation; reads skip expired
-- rows and writes purge a bounded batch. Payloads are small JSON objects
-- with public fields only: no email, GPS or tokens. Deleting an account
-- row removes its inbox; soft-deleted accounts are erased by the worker.
CREATE TABLE account_notifications (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    event_key TEXT NOT NULL CONSTRAINT account_notifications_event_key_check
        CHECK (char_length(event_key) BETWEEN 1 AND 200),
    kind TEXT NOT NULL CONSTRAINT account_notifications_kind_check
        CHECK (char_length(kind) <= 64 AND kind ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    payload JSONB NOT NULL CONSTRAINT account_notifications_payload_check
        CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 2048),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    read_at TIMESTAMPTZ,
    CONSTRAINT account_notifications_event_unique UNIQUE (account_id, event_key),
    CONSTRAINT account_notifications_expiry_check CHECK (expires_at > created_at)
);

-- Newest-first feed and its (created_at, id) keyset cursor.
CREATE INDEX account_notifications_feed_idx
    ON account_notifications (account_id, created_at DESC, id DESC);

-- Bounded oldest-first purge of expired rows.
CREATE INDEX account_notifications_expiry_idx
    ON account_notifications (expires_at);
