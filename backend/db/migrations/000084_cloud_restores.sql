-- PRM04-T05 idempotent restore receipts. Append-only; receipts carry only
-- operation metadata, never private payloads. Restores never resurrect
-- erased UUIDs; pre-restore revisions stay recoverable via the change feed.
CREATE TABLE cloud_restores(
 account_id UUID NOT NULL REFERENCES accounts(id),
 restore_id UUID NOT NULL,
 snapshot_id UUID NOT NULL,
 pre_revision BIGINT NOT NULL CHECK(pre_revision>=0),
 post_revision BIGINT NOT NULL CHECK(post_revision>=0),
 receipt JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, restore_id)
);
CREATE INDEX cloud_restores_snapshot_idx ON cloud_restores(account_id, snapshot_id);
