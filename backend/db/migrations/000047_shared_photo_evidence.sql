-- Separate shared capture lane; legacy object/observation claims remain exclusive.
ALTER TABLE evidence_objects ADD COLUMN bound_capture_id uuid;
ALTER TABLE evidence_objects ADD CONSTRAINT evidence_binding_lane_exclusive
 CHECK (bound_capture_id IS NULL OR bound_observation_id IS NULL);
