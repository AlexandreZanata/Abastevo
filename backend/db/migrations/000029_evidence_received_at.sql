-- 000029: forward 24 h copy deadline (P15-T04, B-BR-M01/M04).
-- Append-only: new nullable first-receipt stamp plus sweep index,
-- no history rewrite. received_at records when object bytes first
-- landed (completion intent); the copy deadline is received_at +
-- 24 h for every app-owned copy, replacing the 14-day default and
-- 30-day case extension for photo bytes (those columns stay as
-- audit history, the sweeper no longer honors them). Existing rows
-- backfill from created_at, so old media goes overdue on the next
-- sweep pass (purge-old-media rollout, no deadline reset).
ALTER TABLE evidence_objects
    ADD COLUMN received_at TIMESTAMPTZ NULL;
UPDATE evidence_objects
    SET received_at = created_at
    WHERE received_at IS NULL;
CREATE INDEX evidence_objects_received_retention_idx
    ON evidence_objects (received_at)
    WHERE final_deleted_at IS NULL;
