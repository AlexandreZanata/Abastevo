-- Preserve the original unzoned source clock; legacy/date-only rows stay NULL.
ALTER TABLE quality_assays ADD COLUMN source_observed_at TEXT;
ALTER TABLE quality_assays ADD CONSTRAINT quality_assays_source_clock_check CHECK(
 source_observed_at IS NULL OR (
 length(source_observed_at)=19 AND source_observed_at ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2} (0[0-9]|1[0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$'
 AND left(source_observed_at,10)=to_char(observed_on,'YYYY-MM-DD')));
