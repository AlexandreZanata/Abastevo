-- Owned by directory. Modules must not import another module's generated
-- package; cross-module reads use owned queries documented in DATA_MODEL.

-- name: CreateStation :one
INSERT INTO directory_stations (id, display_name, address, municipality_code, state)
VALUES (@id, @display_name, @address, @municipality_code, @state)
RETURNING id, display_name, municipality_code, state, status, created_at;

-- name: GetStation :one
SELECT id, display_name, address, municipality_code, state, status,
    ST_AsText(current_point) AS current_point_wkt, current_quality,
    current_revision_id, created_at
FROM directory_stations
WHERE id = @id;

-- name: ResolveActiveIdentifier :one
SELECT s.id, s.display_name, s.municipality_code, s.state, s.status,
    ST_AsText(s.current_point) AS current_point_wkt, s.current_quality,
    s.current_revision_id, s.created_at
FROM directory_identifiers AS i
JOIN directory_stations AS s ON s.id = i.station_id
WHERE i.kind = @kind AND i.normalized_value = @value AND i.valid_to IS NULL;

-- name: CreateIdentifier :one
INSERT INTO directory_identifiers (id, station_id, kind, normalized_value, source_revision_id)
VALUES (@id, @station_id, @kind, @value, @source_revision_id)
ON CONFLICT (kind, normalized_value) WHERE valid_to IS NULL DO NOTHING
RETURNING station_id;

-- name: RetireIdentifier :execrows
UPDATE directory_identifiers
SET valid_to = now()
WHERE kind = @kind AND normalized_value = @value AND valid_to IS NULL;

-- name: CountActiveIdentifiers :one
SELECT count(*) FROM directory_identifiers WHERE valid_to IS NULL;

-- name: CountStations :one
SELECT count(*) FROM directory_stations;

-- name: CreateLocationRevision :one
INSERT INTO directory_location_revisions
    (id, station_id, point, quality, provider, source_reference, supersedes_id)
VALUES (
    @id, @station_id,
    ST_GeogFromText(NULLIF(@point_wkt, '')),
    @quality, @provider, @source_reference, @supersedes_id
)
RETURNING id, station_id, ST_AsText(point) AS point_wkt, quality,
    provider, source_reference, obtained_at, supersedes_id;

-- name: ListLocationRevisions :many
SELECT id, station_id, ST_AsText(point) AS point_wkt, quality,
    provider, source_reference, obtained_at, supersedes_id
FROM directory_location_revisions
WHERE station_id = @station_id
ORDER BY obtained_at ASC, id ASC;

-- name: UpdateStationProjection :exec
UPDATE directory_stations
SET current_point = ST_GeogFromText(NULLIF(@point_wkt, '')),
    current_quality = @quality,
    current_revision_id = @revision_id
WHERE id = @id;

-- name: SearchStations :many
SELECT id, display_name, address, municipality_code, state, status,
    ST_AsText(current_point) AS current_point_wkt, current_quality,
    current_revision_id, created_at
FROM directory_stations
WHERE (@state::text = '' OR state = @state)
    AND (@municipality::text = '' OR municipality_code = @municipality)
    AND (@q::text = '' OR display_name ILIKE '%' || @q || '%')
    AND (@after_id::text = '' OR id::text > @after_id::text)
ORDER BY id::text ASC
LIMIT @limit_plus_one::int;

-- name: NearbyStations :many
SELECT id, display_name, address, municipality_code, state, status,
    ST_AsText(current_point) AS current_point_wkt, current_quality,
    current_revision_id, created_at,
    ST_Distance(current_point, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography) AS distance_m
FROM directory_stations
WHERE current_point IS NOT NULL
    AND ST_DWithin(current_point, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography, @radius_m::int)
    AND (@has_cursor::boolean = FALSE OR
        (ST_Distance(current_point, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography) > @after_dist::float8) OR
        (ST_Distance(current_point, ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography) = @after_dist::float8 AND id::text > @after_id::text))
ORDER BY distance_m ASC, id::text ASC
LIMIT @limit_plus_one::int;

-- name: StationCNPJ :one
SELECT normalized_value FROM directory_identifiers
WHERE station_id = @station_id AND kind = 'CNPJ' AND valid_to IS NULL;

-- Server-side distances for location intake (P16-T03B): exact fixes
-- never persist, so proximity derives in PostGIS at submit time and
-- only bands survive. NULL point or unknown station yields no row.

-- name: FixDistanceM :one
SELECT ST_Distance(current_point,
    ST_SetSRID(ST_MakePoint(@lon::float8, @lat::float8), 4326)::geography) AS distance_m,
    current_quality
FROM directory_stations
WHERE id = @station_id AND current_point IS NOT NULL;

-- name: StationPairDistanceM :one
SELECT ST_Distance(a.current_point, b.current_point) AS distance_m
FROM directory_stations AS a
JOIN directory_stations AS b ON b.id = @other_id
WHERE a.id = @station_id
    AND a.current_point IS NOT NULL AND b.current_point IS NOT NULL;

-- name: StationsMissingProjection :many
SELECT s.id, s.display_name, s.address, s.municipality_code, s.state
FROM directory_stations AS s
WHERE s.current_point IS NULL
    AND NOT EXISTS (
        SELECT 1 FROM directory_location_revisions AS r
        WHERE r.station_id = s.id
    )
ORDER BY s.created_at ASC, s.id ASC
LIMIT @limit_n;
