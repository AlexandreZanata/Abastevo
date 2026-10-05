-- 000031: registry source runs and staged assertions (P25-T02).
-- Append-only: staging only, never publication. Runs ledger every bounded
-- snapshot attempt with checksum/parser version/completeness; assertions
-- hold one validated source row each with a stable source key. Identical
-- replay is idempotent via the (source, source_key, checksum) unique key;
-- corrections arrive as new checksums. Publishers (P25-T04) read only
-- complete runs; failed/quarantined runs stay invisible to them. No
-- station, price, trust or private data here.
CREATE TABLE registry_source_runs (
    id UUID PRIMARY KEY,
    source TEXT NOT NULL,
    snapshot_identity TEXT NOT NULL,
    checksum TEXT NOT NULL,
    parser_version TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'running',
    accepted BIGINT NOT NULL DEFAULT 0,
    duplicates BIGINT NOT NULL DEFAULT 0,
    rejected BIGINT NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    CONSTRAINT registry_source_runs_source_check
        CHECK (source IN ('registry-csv', 'registry-api')),
    CONSTRAINT registry_source_runs_state_check
        CHECK (state IN ('running', 'complete', 'failed', 'quarantined'))
);
CREATE UNIQUE INDEX registry_source_runs_snapshot_unique
    ON registry_source_runs (source, snapshot_identity);
CREATE TABLE registry_assertions (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES registry_source_runs (id) ON DELETE RESTRICT,
    source TEXT NOT NULL,
    source_key TEXT NOT NULL,
    checksum TEXT NOT NULL,
    station_id UUID REFERENCES directory_stations (id) ON DELETE RESTRICT,
    display_name TEXT NOT NULL,
    address JSONB NOT NULL DEFAULT '{}'::jsonb,
    municipality_code TEXT,
    state CHAR(2),
    auth_state TEXT NOT NULL DEFAULT 'unknown',
    operation TEXT NOT NULL DEFAULT 'unknown',
    eligibility TEXT NOT NULL DEFAULT 'pending',
    location_quality TEXT NOT NULL DEFAULT 'unknown',
    source_reference TEXT NOT NULL DEFAULT '',
    effective_date DATE,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_by UUID REFERENCES registry_assertions (id) ON DELETE RESTRICT,
    CONSTRAINT registry_assertions_auth_state_check
        CHECK (auth_state IN ('unknown', 'authorized', 'suspended', 'revoked')),
    CONSTRAINT registry_assertions_eligibility_check
        CHECK (eligibility IN ('ineligible', 'pending', 'eligible', 'withdrawn')),
    CONSTRAINT registry_assertions_location_check
        CHECK (location_quality IN ('unknown', 'city-centroid', 'reviewed'))
);
CREATE UNIQUE INDEX registry_assertions_replay_unique
    ON registry_assertions (source, source_key, checksum);
CREATE INDEX registry_assertions_run_idx
    ON registry_assertions (run_id);
