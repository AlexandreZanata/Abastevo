-- 000139: national map location tiers (B-BR-MAP-02). Append-only.
-- Widens the stored location quality ladder with the three open-data pin
-- tiers below reviewed: osm-mapped (OSM fuel POI bound by CNPJ tag or exact
-- address), anp-reported (official ANP Revendedores API point) and
-- address-point (IBGE CNEFE establishment at the exact street and number).
-- city-centroid and unknown stay recorded-only and never project a point.
-- Existing rows already satisfy the wider constraints; no backfill.
ALTER TABLE directory_location_revisions
    DROP CONSTRAINT directory_location_revisions_quality_check,
    ADD CONSTRAINT directory_location_revisions_quality_check
        CHECK (quality IN ('unknown', 'city-centroid', 'reviewed',
                           'osm-mapped', 'anp-reported', 'address-point'));

-- A projected point always belongs to a pin tier: unknown and city-centroid
-- never carry a current point. NOT VALID + VALIDATE keeps the lock short on
-- the national table (the validation scan holds SHARE UPDATE EXCLUSIVE).
ALTER TABLE directory_stations
    ADD CONSTRAINT directory_stations_current_quality_check
        CHECK (current_quality IS NULL OR current_quality IN
               ('unknown', 'city-centroid', 'reviewed',
                'osm-mapped', 'anp-reported', 'address-point')) NOT VALID,
    ADD CONSTRAINT directory_stations_current_point_tier_check
        CHECK (current_point IS NULL OR current_quality IN
               ('reviewed', 'osm-mapped', 'anp-reported', 'address-point')) NOT VALID;
ALTER TABLE directory_stations VALIDATE CONSTRAINT directory_stations_current_quality_check;
ALTER TABLE directory_stations VALIDATE CONSTRAINT directory_stations_current_point_tier_check;
