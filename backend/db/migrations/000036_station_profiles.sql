-- 000036: station operator revisions and public profiles (P30-T02).
-- Append-only: operator revisions link the stable station ID to its
-- effective full CNPJ with source/validity (branch vs matrix resolved
-- by source evidence, never name matching); station profiles carry the
-- versioned public business projection (empty while unclaimed). No
-- second station registry: station_id references directory_stations.
-- No prices, grants, private proofs or personal data here.
CREATE TABLE station_operator_revisions (
    id UUID PRIMARY KEY,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    cnpj TEXT NOT NULL,
    source TEXT NOT NULL,
    source_reference TEXT NOT NULL DEFAULT '',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ,
    CONSTRAINT station_operator_revisions_source_check
        CHECK (source IN ('registry', 'dou', 'review'))
);
CREATE INDEX station_operator_revisions_station_idx
    ON station_operator_revisions (station_id) WHERE valid_to IS NULL;
CREATE UNIQUE INDEX station_operator_revisions_open_unique
    ON station_operator_revisions (station_id) WHERE valid_to IS NULL;
CREATE TABLE station_profiles (
    station_id UUID PRIMARY KEY REFERENCES directory_stations (id) ON DELETE RESTRICT,
    policy_version TEXT NOT NULL,
    projection JSONB NOT NULL DEFAULT '{}'::jsonb,
    revision INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
