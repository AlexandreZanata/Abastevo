-- 000011: evidence retention bookkeeping (P05-T05). Append-only: new
-- nullable columns plus sweep indexes, no backfill of history.
-- quarantine_deleted_at records when the original upload left object
-- storage (post-sanitization, rejection, expiry or the 24 h hard cap).
-- final_deleted_at records when sanitized bytes left storage (14-day
-- default, substantiated case extension, 30-day absolute cap); the
-- object row with its duplicate-signal hashes survives to 90 days.
-- retention_extended_until/reason are written only by a reviewed case
-- command (moderation, P07); the sweeper honors them read-only.
ALTER TABLE evidence_sessions
    ADD COLUMN quarantine_deleted_at TIMESTAMPTZ NULL;
ALTER TABLE evidence_objects
    ADD COLUMN final_deleted_at TIMESTAMPTZ NULL,
    ADD COLUMN retention_extended_until TIMESTAMPTZ NULL,
    ADD COLUMN retention_extended_reason TEXT NOT NULL DEFAULT '';
CREATE INDEX evidence_sessions_sweep_idx
    ON evidence_sessions (status, created_at)
    WHERE quarantine_deleted_at IS NULL;
CREATE INDEX evidence_objects_final_retention_idx
    ON evidence_objects (created_at)
    WHERE final_deleted_at IS NULL;
CREATE INDEX evidence_objects_created_idx
    ON evidence_objects (created_at);
CREATE INDEX evidence_objects_dhash_idx
    ON evidence_objects (dhash, created_at);
