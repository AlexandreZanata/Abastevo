-- RVA01: durable admission independent of URL replay; no media bytes in DB.
CREATE TABLE evidence_upload_transfers (
 session_id UUID PRIMARY KEY REFERENCES evidence_sessions(id) ON DELETE RESTRICT,
 attempts INTEGER NOT NULL DEFAULT 1 CHECK(attempts BETWEEN 1 AND 5),
 lease_nonce TEXT NOT NULL,
 lease_until TIMESTAMPTZ,
 received_at TIMESTAMPTZ,
 actual_bytes BIGINT NOT NULL DEFAULT 0 CHECK(actual_bytes BETWEEN 0 AND 3145728)
);
