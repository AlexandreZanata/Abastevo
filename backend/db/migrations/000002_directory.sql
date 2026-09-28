-- 000002: canonical station directory (P02-T03). Append-only.
-- Stations keep a source-independent canonical ID; identifiers (CNPJ) are
-- historied rows with a partial unique index on active values so concurrent
-- resolves converge on one station. Locations are append-only revisions with
-- an explicit nullable point (missing stays missing, B-BR-014); the stations
-- row carries the single reviewed projection.
CREATE TABLE directory_stations (
    id UUID PRIMARY KEY,
    display_name TEXT NOT NULL,
    address JSONB NOT NULL DEFAULT '{}'::jsonb,
    municipality_code TEXT,
    state CHAR(2),
    status TEXT NOT NULL DEFAULT 'active',
    current_point geography(Point, 4326),
    current_quality TEXT,
    current_revision_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE directory_identifiers (
    id UUID PRIMARY KEY,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    kind TEXT NOT NULL,
    normalized_value TEXT NOT NULL,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ,
    source_revision_id TEXT,
    CONSTRAINT directory_identifiers_kind_check CHECK (kind IN ('CNPJ'))
);
CREATE UNIQUE INDEX directory_identifiers_active_unique
    ON directory_identifiers (kind, normalized_value) WHERE valid_to IS NULL;
CREATE INDEX directory_identifiers_station_idx
    ON directory_identifiers (station_id) WHERE valid_to IS NULL;
CREATE TABLE directory_location_revisions (
    id UUID PRIMARY KEY,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    point geography(Point, 4326),
    quality TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT '',
    source_reference TEXT NOT NULL DEFAULT '',
    obtained_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    supersedes_id UUID REFERENCES directory_location_revisions (id) ON DELETE RESTRICT,
    CONSTRAINT directory_location_revisions_quality_check
        CHECK (quality IN ('unknown', 'city-centroid', 'reviewed'))
);
CREATE INDEX directory_stations_current_point_idx
    ON directory_stations USING GIST (current_point);
CREATE INDEX directory_stations_municipality_idx
    ON directory_stations (municipality_code, state);
