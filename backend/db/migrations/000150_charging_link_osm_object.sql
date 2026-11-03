-- EVC-04: distinguish exact, source-backed OSM identity from manual and
-- name/proximity decisions. Existing decision rows retain their methods.
ALTER TABLE charging_place_station_links
    DROP CONSTRAINT charging_place_station_links_method_check,
    ADD CONSTRAINT charging_place_station_links_method_check
        CHECK (method IN ('CNPJ', 'ADDRESS_NAME', 'MANUAL', 'OSM_OBJECT'));
