-- Read ordering for batches. Batches are immutable; stored_at records
-- admission order for latest-first reads. Existing rows backfill with
-- statement timestamp (ordering among backfilled rows is unspecified and
-- must not be relied upon for correctness; callers specify revisions
-- explicitly when exactness matters).
ALTER TABLE charging_batches ADD COLUMN stored_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX charging_batches_profile_stored_idx ON charging_batches(profile_id, stored_at DESC);
