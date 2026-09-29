-- 000005: operation idempotency (P03-T04). Append-only.
-- One row per contributor, method, route template and client key. The row
-- is reserved before the business write and completed inside the same
-- transaction, so retries replay the stored outcome instead of executing
-- twice. A NULL response means reserved but not yet completed; expired rows
-- behave as absent. Only the request hash is stored, never the body.
CREATE TABLE identity_idempotency (
    contributor_id UUID NOT NULL REFERENCES identity_contributors (id) ON DELETE RESTRICT,
    method TEXT NOT NULL,
    route TEXT NOT NULL,
    key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_code INTEGER,
    response_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (contributor_id, method, route, key)
);
CREATE INDEX identity_idempotency_expiry_idx ON identity_idempotency (expires_at);
