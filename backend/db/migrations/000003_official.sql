-- 000003: official ANP revisions and immutable prices (P02-T06). Append-only.
-- Imports stage rows under a staging revision; only a validated publish
-- switches the per-survey pointer. Retried identical bytes are a no-op via
-- the run identity; corrected bytes open a new revision that supersedes.
-- Quarantine stays as counts, reasons and bounded samples inside the run;
-- staged rows of failed or under-review revisions are never reachable
-- through the pointer.
CREATE TABLE official_import_runs (
    id UUID PRIMARY KEY,
    source_url TEXT NOT NULL,
    source_checksum TEXT NOT NULL,
    parser_version TEXT NOT NULL,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status TEXT NOT NULL DEFAULT 'running',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    row_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT official_import_runs_status_check
        CHECK (status IN ('running', 'completed', 'failed'))
);
CREATE UNIQUE INDEX official_import_runs_identity_unique
    ON official_import_runs (source_url, source_checksum, parser_version);
CREATE TABLE official_revisions (
    id UUID PRIMARY KEY,
    import_run_id UUID NOT NULL REFERENCES official_import_runs (id) ON DELETE RESTRICT,
    survey_start DATE NOT NULL,
    survey_end DATE NOT NULL,
    published_at TIMESTAMPTZ,
    supersedes_revision_id UUID REFERENCES official_revisions (id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'staging',
    CONSTRAINT official_revisions_status_check
        CHECK (status IN ('staging', 'published', 'failed', 'needs-review')),
    CONSTRAINT official_revisions_dates_check CHECK (survey_start <= survey_end)
);
CREATE TABLE official_current_revisions (
    survey_start DATE NOT NULL,
    survey_end DATE NOT NULL,
    revision_id UUID NOT NULL REFERENCES official_revisions (id) ON DELETE RESTRICT,
    switched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (survey_start, survey_end)
);
CREATE TABLE official_station_prices (
    id UUID PRIMARY KEY,
    revision_id UUID NOT NULL REFERENCES official_revisions (id) ON DELETE RESTRICT,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    fuel_product TEXT NOT NULL,
    unit TEXT NOT NULL,
    amount_milli_brl BIGINT NOT NULL CHECK (amount_milli_brl >= 1),
    raw_price_text TEXT NOT NULL,
    collected_on DATE NOT NULL,
    source_row INTEGER NOT NULL,
    CONSTRAINT official_station_prices_product_check CHECK (fuel_product IN
        ('ETHANOL', 'GASOLINE_REGULAR', 'GASOLINE_ADDITIVED', 'DIESEL_S500',
         'DIESEL_S10', 'CNG', 'LPG_P13')),
    CONSTRAINT official_station_prices_unit_check CHECK (unit IN ('L', 'M3', 'KG_13'))
);
CREATE UNIQUE INDEX official_station_prices_row_unique
    ON official_station_prices (revision_id, source_row);
CREATE INDEX official_station_prices_natural_idx
    ON official_station_prices
    (revision_id, station_id, fuel_product, unit, collected_on);
CREATE TABLE official_summary_prices (
    id UUID PRIMARY KEY,
    revision_id UUID NOT NULL REFERENCES official_revisions (id) ON DELETE RESTRICT,
    scope TEXT NOT NULL,
    location_code TEXT NOT NULL,
    fuel_product TEXT NOT NULL,
    unit TEXT NOT NULL,
    mean NUMERIC,
    min_amount NUMERIC,
    max_amount NUMERIC,
    std_dev NUMERIC,
    station_count INTEGER,
    raw_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT official_summary_prices_scope_check
        CHECK (scope IN ('national', 'state', 'municipal'))
);
