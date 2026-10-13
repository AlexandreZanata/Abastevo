-- Singleton operational observation: no contributor or media identifiers.
CREATE TABLE evidence_sweep_observation (
    singleton boolean PRIMARY KEY CHECK (singleton),
    epoch bigint NOT NULL CHECK (epoch > 0),
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    healthy boolean NOT NULL DEFAULT false,
    last_success_at timestamptz,
    CHECK (NOT healthy OR (finished_at IS NOT NULL AND last_success_at = finished_at))
);
-- Bounded distinct-session capacity scans exclude terminal copy history.
CREATE INDEX evidence_final_output_capacity_idx
    ON evidence_final_outputs (session_id) WHERE state != 'PURGED';
CREATE INDEX evidence_object_capacity_idx
    ON evidence_objects (session_id) WHERE final_deleted_at IS NULL;
