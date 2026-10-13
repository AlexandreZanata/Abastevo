-- PRM04-T02 private snapshot inventory. Append-only; object bytes live in
-- private storage under snapshots/, never in the database. Metadata-first
-- fenced receipts: PENDING rows are tracked orphans until verified READY.
-- Erased-UUID and revocation fences are never cleaned here.
CREATE TABLE cloud_snapshots(
 account_id UUID NOT NULL REFERENCES accounts(id),
 snapshot_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>=0),
 epoch BIGINT NOT NULL CHECK(epoch>0),
 consent_revision BIGINT NOT NULL CHECK(consent_revision>=0),
 byte_size BIGINT NOT NULL CHECK(byte_size>0),
 checksum TEXT NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),
 status TEXT NOT NULL CHECK(status IN ('PENDING','READY','FAILED')),
 object_key TEXT NOT NULL CHECK(object_key LIKE 'snapshots/%'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 verified_at TIMESTAMPTZ,
 PRIMARY KEY(account_id, snapshot_id)
);
CREATE INDEX cloud_snapshots_ready_idx ON cloud_snapshots(account_id, revision) WHERE status='READY';
CREATE INDEX cloud_snapshots_retention_idx ON cloud_snapshots(account_id, created_at);
