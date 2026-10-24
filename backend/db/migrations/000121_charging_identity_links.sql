-- 000121: reviewed charging-place/station links and separate access points.
-- Append-only decisions: one active decision per place and one active point
-- per station/kind. Supersession is explicit; no distance-only merge occurs.
CREATE TABLE charging_place_station_links (
    id UUID PRIMARY KEY,
    place_id UUID NOT NULL REFERENCES charging_places(id) ON DELETE RESTRICT,
    station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('REVIEWED', 'UNRESOLVED', 'REJECTED')),
    method TEXT NOT NULL CHECK (method IN ('CNPJ', 'ADDRESS_NAME', 'MANUAL')),
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    reviewer_ref TEXT NOT NULL CHECK (octet_length(reviewer_ref) BETWEEN 1 AND 200),
    reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_by UUID REFERENCES charging_place_station_links(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT charging_links_no_self_supersede CHECK (superseded_by IS NULL OR superseded_by <> id)
);
CREATE UNIQUE INDEX charging_links_one_active_place
    ON charging_place_station_links (place_id) WHERE superseded_by IS NULL;
CREATE INDEX charging_links_station_idx
    ON charging_place_station_links (station_id) WHERE superseded_by IS NULL;

CREATE TABLE directory_access_points (
    id UUID PRIMARY KEY,
    station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('DISPLAY', 'ENTRANCE', 'ROAD_SNAP')),
    point GEOGRAPHY(Point, 4326) NOT NULL,
    quality TEXT NOT NULL CHECK (quality IN ('reviewed', 'osm-mapped')),
    source_reference TEXT NOT NULL CHECK (octet_length(source_reference) BETWEEN 1 AND 256),
    reviewer_ref TEXT NOT NULL CHECK (octet_length(reviewer_ref) BETWEEN 1 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_by UUID REFERENCES directory_access_points(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT access_points_no_self_supersede CHECK (superseded_by IS NULL OR superseded_by <> id)
);
CREATE UNIQUE INDEX directory_access_points_one_active_kind
    ON directory_access_points (station_id, kind) WHERE superseded_by IS NULL;
CREATE INDEX directory_access_points_station_idx
    ON directory_access_points (station_id);
