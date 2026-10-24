-- 000122: public map overlay generations, source-specific assertions and
-- revocations. Generations are append-only; activation never rewrites prior
-- facts. Prices stay independent from native routing packages.
CREATE TABLE map_generations (
    id UUID PRIMARY KEY,
    generation_key TEXT NOT NULL,
    format TEXT NOT NULL,
    product_class TEXT NOT NULL,
    visibility TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    profile_sha256 TEXT NOT NULL,
    rights_sha256 TEXT NOT NULL,
    source_revision TEXT NOT NULL,
    assertions_sha256 TEXT NOT NULL,
    content_sha256 TEXT NOT NULL,
    input_count INTEGER NOT NULL,
    accepted_count INTEGER NOT NULL,
    duplicate_count INTEGER NOT NULL,
    quarantined_count INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ,
    superseded_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT map_generations_format_check CHECK (format = 'abastevo.map-prepared-v1'),
    CONSTRAINT map_generations_public_check CHECK (product_class = 'PUBLIC_OVERLAY' AND visibility = 'PUBLIC'),
    CONSTRAINT map_generations_hash_check CHECK (
        length(profile_sha256) = 64 AND length(rights_sha256) = 64 AND
        length(assertions_sha256) = 64 AND length(content_sha256) = 64
    ),
    CONSTRAINT map_generations_counts_check CHECK (
        input_count >= 0 AND accepted_count >= 0 AND duplicate_count >= 0 AND quarantined_count >= 0 AND
        input_count = accepted_count + duplicate_count + quarantined_count
    )
);
CREATE UNIQUE INDEX map_generations_key_unique ON map_generations (generation_key);
CREATE UNIQUE INDEX map_generations_one_active ON map_generations (status) WHERE status = 'ACTIVE';
CREATE INDEX map_generations_content_idx ON map_generations (content_sha256);

CREATE TABLE map_assertions (
    generation_id UUID NOT NULL REFERENCES map_generations (id) ON DELETE RESTRICT,
    assertion_id UUID NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('ANP', 'COMMUNITY', 'OSM', 'OCM')),
    kind TEXT NOT NULL CHECK (kind IN ('FUEL_STATION', 'CHARGING_SITE', 'COMBINED_SITE', 'REFERENCE_POI')),
    name TEXT NOT NULL,
    station_id UUID,
    cnpj TEXT,
    place_id UUID,
    equipment_id UUID,
    connector_id UUID,
    osm_type TEXT CHECK (osm_type IS NULL OR osm_type IN ('node', 'way', 'relation')),
    osm_id BIGINT,
    latitude_deg NUMERIC(9, 7),
    longitude_deg NUMERIC(10, 7),
    point GEOGRAPHY(Point, 4326),
    quality TEXT NOT NULL CHECK (quality IN ('reviewed', 'osm-mapped', 'unknown', 'city-centroid')),
    display_point TEXT,
    entrance_latitude_deg NUMERIC(9, 7),
    entrance_longitude_deg NUMERIC(10, 7),
    services TEXT[] NOT NULL,
    charging JSONB,
    clocks JSONB NOT NULL,
    rights JSONB NOT NULL,
    capabilities JSONB NOT NULL,
    assertion_sha256 TEXT NOT NULL CHECK (length(assertion_sha256) = 64),
    PRIMARY KEY (generation_id, assertion_id),
    CONSTRAINT map_assertions_osm_pair CHECK ((osm_type IS NULL) = (osm_id IS NULL))
);
CREATE INDEX map_assertions_point_idx ON map_assertions USING GIST (point);
CREATE INDEX map_assertions_station_idx ON map_assertions (station_id) WHERE station_id IS NOT NULL;
CREATE INDEX map_assertions_source_kind_idx ON map_assertions (source, kind);
CREATE INDEX map_assertions_osm_idx ON map_assertions (osm_type, osm_id) WHERE osm_id IS NOT NULL;

CREATE TABLE map_assertion_prices (
    generation_id UUID NOT NULL,
    assertion_id UUID NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('ANP', 'COMMUNITY')),
    station_id UUID NOT NULL,
    product TEXT NOT NULL,
    unit TEXT NOT NULL,
    amount_milli_brl BIGINT NOT NULL CHECK (amount_milli_brl BETWEEN 1 AND 1000000),
    observed_at DATE NOT NULL,
    period_start DATE,
    period_end DATE,
    PRIMARY KEY (generation_id, assertion_id, source, product, unit),
    FOREIGN KEY (generation_id, assertion_id) REFERENCES map_assertions (generation_id, assertion_id) ON DELETE RESTRICT,
    CONSTRAINT map_assertion_prices_period_check CHECK ((period_start IS NULL) = (period_end IS NULL)),
    CONSTRAINT map_assertion_prices_period_order CHECK (period_start IS NULL OR period_start <= period_end)
);
CREATE INDEX map_assertion_prices_query_idx
    ON map_assertion_prices (generation_id, source, product, unit, station_id);

-- Revocations are append-only and survive later generations because
-- assertion ids are stable. A revoked record stays in history but is never
-- served again.
CREATE TABLE map_assertion_revocations (
    id UUID PRIMARY KEY,
    assertion_id UUID NOT NULL,
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 500),
    source_reference TEXT NOT NULL CHECK (octet_length(source_reference) BETWEEN 1 AND 256),
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX map_assertion_revocations_assertion_idx
    ON map_assertion_revocations (assertion_id);

CREATE TABLE map_generation_revocations (
    id UUID PRIMARY KEY,
    generation_id UUID NOT NULL REFERENCES map_generations (id) ON DELETE RESTRICT,
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 500),
    source_reference TEXT NOT NULL CHECK (octet_length(source_reference) BETWEEN 1 AND 256),
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX map_generation_revocations_generation_idx
    ON map_generation_revocations (generation_id);
