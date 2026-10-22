-- Owned by official. MEX-28 server-side ANP weekly ranking; no HTTP route.

-- name: CurrentRankingRevision :one
SELECT r.id, r.survey_start, r.survey_end, r.published_at,
    run.source_url, run.source_checksum
FROM official_current_revisions AS c
JOIN official_revisions AS r ON r.id = c.revision_id
JOIN official_import_runs AS run ON run.id = r.import_run_id
WHERE c.survey_start = @survey_week_start::date
    AND r.status = 'published'
ORDER BY c.switched_at DESC
LIMIT 1;

-- One latest observation per station, grouped by located municipality.
-- name: RankingMunicipalityAggregates :many
WITH latest AS (
    SELECT DISTINCT ON (p.station_id)
        p.station_id, p.amount_milli_brl
    FROM official_station_prices AS p
    WHERE p.revision_id = @revision_id::uuid
        AND p.fuel_product = @fuel_product::text
        AND p.unit = @unit::text
    ORDER BY p.station_id, p.collected_on DESC, p.id
)
SELECT d.state AS state,
    d.municipality_code AS municipality_code,
    SUM(l.amount_milli_brl)::bigint AS sum_milli_brl,
    COUNT(*)::bigint AS observation_count,
    MIN(l.amount_milli_brl)::bigint AS min_milli_brl,
    MAX(l.amount_milli_brl)::bigint AS max_milli_brl
FROM latest AS l
JOIN directory_stations AS d ON d.id = l.station_id
WHERE d.state IS NOT NULL
    AND d.municipality_code IS NOT NULL
    AND (@uf::text = '' OR d.state = @uf)
    AND (@municipality_code::text = '' OR d.municipality_code = @municipality_code)
GROUP BY d.state, d.municipality_code
ORDER BY d.state ASC, d.municipality_code ASC;

-- Latest observations whose station has no canonical state/municipality.
-- name: RankingUnlocatedObservations :one
WITH latest AS (
    SELECT DISTINCT ON (p.station_id)
        p.station_id
    FROM official_station_prices AS p
    WHERE p.revision_id = @revision_id::uuid
        AND p.fuel_product = @fuel_product::text
        AND p.unit = @unit::text
    ORDER BY p.station_id, p.collected_on DESC, p.id
)
SELECT COUNT(*)::bigint AS unlocated
FROM latest AS l
JOIN directory_stations AS d ON d.id = l.station_id
WHERE d.state IS NULL OR d.municipality_code IS NULL;

-- Distinct stations present in the revision for the selected geography,
-- independent of the requested product (coverage denominator).
-- name: RankingSourceStations :one
SELECT COUNT(DISTINCT p.station_id)::bigint AS source_stations
FROM official_station_prices AS p
JOIN directory_stations AS d ON d.id = p.station_id
WHERE p.revision_id = @revision_id::uuid
    AND (@uf::text = '' OR d.state = @uf)
    AND (@municipality_code::text = '' OR d.municipality_code = @municipality_code);

-- Raw source rows for the requested product/unit (not deduplicated).
-- name: RankingSourceObservations :one
SELECT COUNT(*)::bigint AS source_observations
FROM official_station_prices AS p
JOIN directory_stations AS d ON d.id = p.station_id
WHERE p.revision_id = @revision_id::uuid
    AND p.fuel_product = @fuel_product::text
    AND p.unit = @unit::text
    AND (@uf::text = '' OR d.state = @uf)
    AND (@municipality_code::text = '' OR d.municipality_code = @municipality_code);

-- Stable station ranking: amount asc/desc, station UUID tie-break.
-- name: RankingStationObservations :many
WITH latest AS (
    SELECT DISTINCT ON (p.station_id)
        p.station_id, p.amount_milli_brl, p.collected_on
    FROM official_station_prices AS p
    WHERE p.revision_id = @revision_id::uuid
        AND p.fuel_product = @fuel_product::text
        AND p.unit = @unit::text
    ORDER BY p.station_id, p.collected_on DESC, p.id
)
SELECT l.station_id, l.amount_milli_brl, l.collected_on,
    d.state AS state, d.municipality_code AS municipality_code
FROM latest AS l
JOIN directory_stations AS d ON d.id = l.station_id
WHERE (@uf::text = '' OR d.state = @uf)
    AND (@municipality_code::text = '' OR d.municipality_code = @municipality_code)
ORDER BY
    CASE WHEN @sort_asc::boolean THEN l.amount_milli_brl ELSE -l.amount_milli_brl END ASC,
    l.station_id ASC
LIMIT @limit_plus_one::int;
