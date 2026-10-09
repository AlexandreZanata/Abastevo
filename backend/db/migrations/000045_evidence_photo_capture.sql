-- An opaque module-boundary reference, not a foreign key to ephemeral receipts.
ALTER TABLE evidence_sessions ADD COLUMN photo_capture_id uuid;
CREATE UNIQUE INDEX evidence_sessions_photo_capture_unique
 ON evidence_sessions(photo_capture_id) WHERE photo_capture_id IS NOT NULL;
