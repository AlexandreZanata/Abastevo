-- One oldest-first source scan serves terminal/expired originals; active
-- upload leases are excluded by a primary-key lookup in the owned query.
CREATE INDEX evidence_original_cleanup_idx
    ON evidence_sessions (created_at, id) WHERE quarantine_deleted_at IS NULL;
