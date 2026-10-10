-- 000051: city-listing covering index (RST-07). Append-only, plain
-- btree, no constraint change: global active-CNPJ uniqueness and stable
-- UUID foreign keys are untouched. Measured on synthetic census-shaped
-- data (disposable PostGIS 18.6/3.6.4): city-dense first-page p95
-- 83.7 ms -> 49.2 ms at 1M rows with no nearby regression, +56 MiB
-- index size, 1.7 s build. Table partitioning stays rejected: reads are
-- single-digit ms at 100k and sub-target at 1M on the indexed baseline.
CREATE INDEX directory_stations_city_covering_idx
    ON directory_stations (state, municipality_code, status, id);
