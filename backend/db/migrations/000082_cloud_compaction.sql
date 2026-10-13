-- PRM03-T04 retention-bounded compaction index. Append-only; preserves
-- cloud_erased_ids and cloud_device_revocations fences. Physical tombstone
-- removal never resurrects: erased UUIDs stay fenced after row cleanup.
CREATE INDEX IF NOT EXISTS cloud_changes_retention_idx ON cloud_changes(account_id, created_at);
