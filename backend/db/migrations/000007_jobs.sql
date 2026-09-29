-- 000007: durable PostgreSQL job queue (P03-T07). Append-only.
-- At-least-once delivery with fencing leases: workers claim queued rows
-- (or expired leases) with SKIP LOCKED, hold a monotonic lease token, and
-- complete or fail only with the current token, so a stale worker can never
-- overwrite. Exactly-once business effects come from consumers keying their
-- idempotent work by dedupe_key, which is unique when present. Poison jobs
-- park in DEAD after max attempts instead of retrying forever; replay is an
-- explicit audited action. No foreign keys: payloads reference business
-- rows loosely so queue operations never couple to module tables.
CREATE TABLE job_queue (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    dedupe_key TEXT,
    status TEXT NOT NULL DEFAULT 'queued',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    lease_token BIGINT NOT NULL DEFAULT 0,
    lease_expires_at TIMESTAMPTZ,
    worker_id TEXT NOT NULL DEFAULT '',
    not_before TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT job_queue_status_check
        CHECK (status IN ('queued', 'leased', 'done', 'failed', 'dead')),
    CONSTRAINT job_queue_attempts_check CHECK (attempts >= 0 AND max_attempts >= 1)
);
CREATE UNIQUE INDEX job_queue_dedupe_unique ON job_queue (dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX job_queue_claim_idx ON job_queue (status, not_before, created_at);
CREATE INDEX job_queue_expiry_idx ON job_queue (lease_expires_at) WHERE status = 'leased';
