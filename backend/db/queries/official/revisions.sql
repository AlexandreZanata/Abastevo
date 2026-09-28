-- Owned by official. Modules must not import another module's generated
-- package; cross-module reads use owned queries documented in DATA_MODEL.

-- name: CreateImportRun :one
INSERT INTO official_import_runs (id, source_url, source_checksum, parser_version)
VALUES (@id, @source_url, @source_checksum, @parser_version)
RETURNING id;

-- name: GetImportRunByIdentity :one
SELECT id, source_url, source_checksum, parser_version, status,
    row_counts, error_summary, started_at, finished_at
FROM official_import_runs
WHERE source_url = @source_url
    AND source_checksum = @source_checksum
    AND parser_version = @parser_version;

-- name: FinishImportRun :exec
UPDATE official_import_runs
SET status = @status,
    row_counts = @row_counts::jsonb,
    error_summary = @error_summary::jsonb,
    finished_at = now()
WHERE id = @id;

-- name: CreateRevision :one
INSERT INTO official_revisions
    (id, import_run_id, survey_start, survey_end, supersedes_revision_id)
VALUES (@id, @import_run_id, @survey_start, @survey_end, @supersedes_revision_id)
RETURNING id;

-- name: GetRevision :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE id = @id;

-- name: SetRevisionPublished :exec
UPDATE official_revisions
SET status = 'published', published_at = now()
WHERE id = @id;

-- name: SetRevisionStatus :exec
UPDATE official_revisions
SET status = @status
WHERE id = @id;

-- name: GetLatestPublishedRevision :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE status = 'published'
ORDER BY published_at DESC, id DESC
LIMIT 1;

-- name: StageStationPrice :one
INSERT INTO official_station_prices
    (id, revision_id, station_id, fuel_product, unit, amount_milli_brl,
     raw_price_text, collected_on, source_row)
VALUES (@id, @revision_id, @station_id, @fuel_product, @unit,
    @amount_milli_brl, @raw_price_text, @collected_on, @source_row)
ON CONFLICT (revision_id, source_row) DO NOTHING
RETURNING id;

-- name: FindConflictingStationPrice :many
SELECT id, source_row, amount_milli_brl
FROM official_station_prices
WHERE revision_id = @revision_id
    AND station_id = @station_id
    AND fuel_product = @fuel_product
    AND unit = @unit
    AND collected_on = @collected_on
    AND source_row <> @source_row
    AND amount_milli_brl <> @amount_milli_brl;

-- name: CountRevisionRows :one
SELECT count(*) FROM official_station_prices WHERE revision_id = @revision_id;

-- name: UpsertCurrentPointer :exec
INSERT INTO official_current_revisions (survey_start, survey_end, revision_id)
VALUES (@survey_start, @survey_end, @revision_id)
ON CONFLICT (survey_start, survey_end)
DO UPDATE SET revision_id = EXCLUDED.revision_id, switched_at = now();

-- name: GetCurrentPointer :one
SELECT revision_id FROM official_current_revisions
WHERE survey_start = @survey_start AND survey_end = @survey_end;
