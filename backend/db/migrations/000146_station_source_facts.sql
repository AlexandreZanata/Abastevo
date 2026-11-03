-- SNA-02A: additive source-name/address metadata. No guessed backfill and no
-- rewrite of immutable historical assertions. An explicit approved job will
-- later rebuild canonical projections from retained complete source evidence.
ALTER TABLE directory_stations
    ADD COLUMN legal_name TEXT,
    ADD COLUMN trade_name TEXT,
    ADD COLUMN address_source TEXT,
    ADD CONSTRAINT directory_station_address_source_check
        CHECK (address_source IN ('anp','rfb','osm','cnefe','municipality'));

ALTER TABLE registry_assertions
    ADD COLUMN legal_name TEXT,
    ADD COLUMN trade_name TEXT,
    ADD COLUMN address_source TEXT,
    ADD COLUMN source_display_name TEXT,
    ADD COLUMN trade_name_provenance JSONB,
    ADD CONSTRAINT registry_assertion_address_source_check
        CHECK (address_source IN ('anp','rfb','osm','cnefe','municipality')),
    ADD CONSTRAINT registry_assertion_name_provenance_check
        CHECK (trade_name_provenance IS NULL OR jsonb_typeof(trade_name_provenance)='object');
