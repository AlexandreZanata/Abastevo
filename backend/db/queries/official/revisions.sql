-- Owned by official. Modules must not import another module's generated
-- package; cross-module reads use owned queries documented in DATA_MODEL.

-- name: CreateImportRun :one
INSERT INTO official_import_runs (id, source_url, source_checksum, parser_version)
VALUES (@id, @source_url, @source_checksum, @parser_version)
RETURNING id;

-- name: CreateImportRunIfAbsent :execrows
INSERT INTO official_import_runs (id, source_url, source_checksum, parser_version)
VALUES (@id, @source_url, @source_checksum, @parser_version)
ON CONFLICT (source_url, source_checksum, parser_version) DO NOTHING;

-- name: GetImportRunByIdentity :one
SELECT id, source_url, source_checksum, parser_version, status,
    row_counts, error_summary, started_at, finished_at
FROM official_import_runs
WHERE source_url = @source_url
    AND source_checksum = @source_checksum
    AND parser_version = @parser_version;

-- name: GetImportRunByID :one
SELECT id, source_url, source_checksum, parser_version, status,
    row_counts, error_summary, started_at, finished_at
FROM official_import_runs
WHERE id = @id;

-- name: FinishImportRun :execrows
UPDATE official_import_runs
SET status = @status,
    row_counts = @row_counts::jsonb,
    error_summary = @error_summary::jsonb,
    finished_at = now()
WHERE id = @id
    AND status = 'running';

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

-- name: GetRevisionByRun :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE import_run_id = @import_run_id;

-- name: LockRevision :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE id = @id
FOR UPDATE;

-- name: SetRevisionPublished :exec
UPDATE official_revisions
SET status = 'published', published_at = now()
WHERE id = @id;

-- name: SetRevisionStatus :exec
UPDATE official_revisions
SET status = @status
WHERE id = @id;

-- name: PublishStagingRevision :execrows
UPDATE official_revisions
SET status = 'published', published_at = now()
WHERE id = @id
    AND status = 'staging';

-- name: RetireStagingRevision :execrows
UPDATE official_revisions
SET status = 'superseded'
WHERE id = @id
    AND status = 'staging';

-- name: RejectStagingRevision :execrows
UPDATE official_revisions
SET status = @status
WHERE id = @id
    AND status = 'staging';

-- name: GetLatestPublishedRevisionForWindow :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE status = 'published'
    AND survey_start = @survey_start
    AND survey_end = @survey_end
ORDER BY published_at DESC, id DESC
LIMIT 1;

-- name: GetLatestPublishedRevisionBeforeWindow :one
SELECT id, import_run_id, survey_start, survey_end, published_at,
    supersedes_revision_id, status
FROM official_revisions
WHERE status = 'published'
    AND survey_end < @survey_start
ORDER BY survey_end DESC, survey_start DESC, published_at DESC, id DESC
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

-- name: GetRevisionRowBySourceRow :one
SELECT id, revision_id, station_id, fuel_product, unit, amount_milli_brl,
    raw_price_text, collected_on, source_row
FROM official_station_prices
WHERE revision_id = @revision_id
    AND source_row = @source_row;

-- name: CountRevisionRows :one
SELECT count(*) FROM official_station_prices WHERE revision_id = @revision_id;

-- name: UpsertCurrentPointer :exec
INSERT INTO official_current_revisions (survey_start, survey_end, revision_id)
VALUES (@survey_start, @survey_end, @revision_id)
ON CONFLICT (survey_start, survey_end)
DO UPDATE SET revision_id = EXCLUDED.revision_id, switched_at = now();

-- name: SwitchCurrentRevision :execrows
INSERT INTO official_current_revisions (survey_start, survey_end, revision_id)
VALUES (@survey_start, @survey_end, @revision_id)
ON CONFLICT (survey_start, survey_end)
DO UPDATE SET revision_id = EXCLUDED.revision_id, switched_at = now();

-- name: GetCurrentPointer :one
SELECT revision_id FROM official_current_revisions
WHERE survey_start = @survey_start AND survey_end = @survey_end;

-- name: StationCurrentPrices :many
-- The latest survey week that contains the station, resolved through that
-- week's current pointer. An older correction must not become "current" just
-- because it was published after the newest week.
WITH latest_station_window AS (
    SELECT r.survey_start, r.survey_end
    FROM official_revisions AS r
    JOIN official_station_prices AS p ON p.revision_id = r.id
    WHERE p.station_id = @station_id
        AND r.status = 'published'
    GROUP BY r.survey_start, r.survey_end
    ORDER BY r.survey_start DESC, r.survey_end DESC
    LIMIT 1
)
SELECT p.id, p.revision_id, p.station_id, p.fuel_product, p.unit,
    p.amount_milli_brl, p.raw_price_text, p.collected_on, p.source_row,
    r.survey_start, r.survey_end, run.source_url, run.source_checksum
FROM official_station_prices AS p
JOIN official_revisions AS r ON r.id = p.revision_id
JOIN official_current_revisions AS c
    ON c.revision_id = r.id
    AND c.survey_start = r.survey_start
    AND c.survey_end = r.survey_end
JOIN official_import_runs AS run ON run.id = r.import_run_id
WHERE p.station_id = @station_id
    AND r.status = 'published'
    AND r.survey_start = (SELECT survey_start FROM latest_station_window)
    AND r.survey_end = (SELECT survey_end FROM latest_station_window)
    AND (@fuel::text = '' OR p.fuel_product = @fuel)
ORDER BY p.fuel_product ASC, p.unit ASC, p.collected_on DESC, p.id ASC;

-- name: StationPriceHistory :many
SELECT p.id, p.revision_id, p.station_id, p.fuel_product, p.unit,
    p.amount_milli_brl, p.raw_price_text, p.collected_on, p.source_row,
    r.survey_start, r.survey_end, run.source_url, run.source_checksum
FROM official_station_prices AS p
JOIN official_revisions AS r ON r.id = p.revision_id
JOIN official_import_runs AS run ON run.id = r.import_run_id
WHERE p.station_id = @station_id
    AND r.status = 'published'
    AND (@fuel::text = '' OR p.fuel_product = @fuel)
    AND (@revision_null::boolean = TRUE OR p.revision_id = @revision_id)
    AND (@has_cursor::boolean = FALSE OR
        (p.collected_on, p.id::text) < (@after_date::date, @after_id::text))
ORDER BY p.collected_on DESC, p.id::text DESC
LIMIT @limit_plus_one::int;

-- name: ExportPublishedRevisions :many
-- Bounded export of the current revision for each published survey window.
SELECT r.id, r.survey_start, r.survey_end, r.published_at,
    r.supersedes_revision_id, run.source_url, run.source_checksum,
    run.parser_version
FROM official_current_revisions AS c
JOIN official_revisions AS r ON r.id = c.revision_id
JOIN official_import_runs AS run ON run.id = r.import_run_id
WHERE r.status = 'published'
    AND r.survey_start >= @from_survey::date
    AND r.survey_end <= @to_survey::date
    AND (@after_id::text = '' OR r.id::text > @after_id::text)
ORDER BY r.id::text ASC
LIMIT @limit_plus_one::int;

-- name: ExportRevisionPrices :many
-- Prices only for a current published revision; staging, failed and superseded
-- revisions never leak into a public snapshot.
SELECT p.id, p.revision_id, p.station_id, p.fuel_product, p.unit,
    p.amount_milli_brl, p.collected_on
FROM official_station_prices AS p
JOIN official_revisions AS r ON r.id = p.revision_id
JOIN official_current_revisions AS c
    ON c.revision_id = r.id
    AND c.survey_start = r.survey_start
    AND c.survey_end = r.survey_end
WHERE p.revision_id = @revision_id
    AND r.status = 'published'
    AND (@after_id::text = '' OR p.id::text > @after_id::text)
ORDER BY p.id::text ASC
LIMIT @limit_plus_one::int;

-- name: LatestPublishedRevision :one
SELECT r.id, r.survey_start, r.survey_end, r.published_at,
    r.supersedes_revision_id, run.source_url, run.source_checksum,
    run.parser_version
FROM official_current_revisions AS c
JOIN official_revisions AS r ON r.id = c.revision_id
JOIN official_import_runs AS run ON run.id = r.import_run_id
WHERE r.status = 'published'
ORDER BY r.survey_end DESC, r.survey_start DESC, r.published_at DESC, r.id DESC
LIMIT 1;
