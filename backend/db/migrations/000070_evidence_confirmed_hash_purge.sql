-- Hash cleanup must retain every destination whose bytes remain unconfirmed.
-- Exclude live object rows from the bounded oldest-first metadata purge scan.
CREATE INDEX evidence_objects_confirmed_purge_idx
    ON evidence_objects (created_at, id) WHERE final_deleted_at IS NOT NULL;
