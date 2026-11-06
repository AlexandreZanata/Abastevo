-- Owned by official. FOR-25 bounded weekly bulletin aggregates.
-- Inputs are only the current published revision of two non-overlapping
-- survey windows; community and charging observations are never joined.

-- name: LatestTwoCurrentPublishedRevisions :one
WITH current AS (
    SELECT r.id, r.survey_start, r.survey_end, r.published_at,
        run.source_url, run.source_checksum, run.parser_version,
        row_number() OVER (
            ORDER BY r.survey_end DESC, r.survey_start DESC,
                r.published_at DESC NULLS LAST, r.id DESC
        ) AS rn
    FROM official_current_revisions AS c
    JOIN official_revisions AS r ON r.id = c.revision_id
    JOIN official_import_runs AS run ON run.id = r.import_run_id
    WHERE r.status = 'published'
)
SELECT
    cur.id AS current_id,
    cur.survey_start AS current_survey_start,
    cur.survey_end AS current_survey_end,
    cur.published_at AS current_published_at,
    cur.source_url AS current_source_url,
    cur.source_checksum AS current_source_checksum,
    cur.parser_version AS current_parser_version,
    prev.id AS previous_id,
    prev.survey_start AS previous_survey_start,
    prev.survey_end AS previous_survey_end,
    prev.published_at AS previous_published_at,
    prev.source_url AS previous_source_url,
    prev.source_checksum AS previous_source_checksum,
    prev.parser_version AS previous_parser_version
FROM current AS cur
JOIN current AS prev ON prev.survey_end < cur.survey_start
WHERE cur.rn = 1
ORDER BY prev.survey_end DESC, prev.survey_start DESC,
    prev.published_at DESC NULLS LAST, prev.id DESC
LIMIT 1;

-- One latest observation per station for the requested fuel/unit, exact sum
-- and coverage per UF. SourceStations is the revision's any-product station
-- denominator in the same UF.
-- name: BulletinRevisionSamples :many
WITH latest AS (
    SELECT DISTINCT ON (p.station_id)
        p.station_id, p.amount_milli_brl
    FROM official_station_prices AS p
    WHERE p.revision_id = @revision_id::uuid
        AND p.fuel_product = @fuel_product::text
        AND p.unit = @unit::text
    ORDER BY p.station_id, p.collected_on DESC, p.id
),
sampled AS (
    SELECT d.state AS uf, l.amount_milli_brl, l.station_id
    FROM latest AS l
    JOIN directory_stations AS d ON d.id = l.station_id
    WHERE d.state IS NOT NULL
),
raw AS (
    SELECT d.state AS uf, COUNT(*)::bigint AS source_observations
    FROM official_station_prices AS p
    JOIN directory_stations AS d ON d.id = p.station_id
    WHERE p.revision_id = @revision_id::uuid
        AND p.fuel_product = @fuel_product::text
        AND p.unit = @unit::text
        AND d.state IS NOT NULL
    GROUP BY d.state
),
sources AS (
    SELECT d.state AS uf, COUNT(DISTINCT p.station_id)::bigint AS source_stations
    FROM official_station_prices AS p
    JOIN directory_stations AS d ON d.id = p.station_id
    WHERE p.revision_id = @revision_id::uuid
        AND d.state IS NOT NULL
    GROUP BY d.state
)
SELECT
    s.uf,
    SUM(s.amount_milli_brl)::text AS sum_milli_brl,
    COUNT(*)::bigint AS observation_count,
    COUNT(DISTINCT s.station_id)::bigint AS sampled_stations,
    COALESCE(r.source_observations, 0)::bigint AS source_observations,
    COALESCE(src.source_stations, 0)::bigint AS source_stations
FROM sampled AS s
LEFT JOIN raw AS r ON r.uf = s.uf
LEFT JOIN sources AS src ON src.uf = s.uf
GROUP BY s.uf, r.source_observations, src.source_stations
ORDER BY s.uf ASC;
