-- 000032: staged assertion coordinates (P25-T04). Append-only: three
-- nullable columns for API-supplied reviewed points. CSV assertions
-- stay NULL (honest unknown, never centroid-fabricated). Reconciliation
-- projects reviewed points through the existing location-revision chain;
-- nothing here publishes by itself.
ALTER TABLE registry_assertions
    ADD COLUMN latitude DOUBLE PRECISION NULL,
    ADD COLUMN longitude DOUBLE PRECISION NULL,
    ADD COLUMN crs TEXT NOT NULL DEFAULT '';
