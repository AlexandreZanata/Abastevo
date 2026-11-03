-- SNA-02B: edition ordering is independent of canonical/curated labels.
ALTER TABLE directory_stations
    ADD COLUMN facts_source_clock TIMESTAMPTZ,
    ADD COLUMN facts_source_checksum TEXT,
    ADD CONSTRAINT directory_fact_projection_clock_check
        CHECK ((facts_source_clock IS NULL AND facts_source_checksum IS NULL)
            OR (facts_source_clock IS NOT NULL AND facts_source_checksum IS NOT NULL AND facts_source_checksum ~ '^[0-9a-f]{64}$'));
CREATE INDEX registry_membership_assertion_idx
    ON registry_run_assertions(assertion_id, run_id);
CREATE INDEX station_corrections_accepted_name_idx
    ON station_corrections(station_id, decided_at DESC, id DESC)
    WHERE field='display_name' AND status='accepted';
