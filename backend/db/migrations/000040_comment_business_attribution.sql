-- 000040: official business attribution on feedback comments (P31-T04).
-- Append-only nullable columns: personal comments stay NULL (existing
-- behavior unchanged); official replies record the station and the
-- exact grant verified at publication. At-time attribution survives
-- later revocation (reads never re-check liveness); new privileged
-- writes always re-check. No backfill: history stays personal.
ALTER TABLE feedback_comments
    ADD COLUMN business_station_id UUID REFERENCES directory_stations (id) ON DELETE RESTRICT,
    ADD COLUMN business_grant_id UUID REFERENCES representation_grants (id) ON DELETE RESTRICT;
CREATE INDEX feedback_comments_business_idx
    ON feedback_comments (business_station_id) WHERE business_station_id IS NOT NULL;
