-- Bound canonical ownership/reprojection lookups to one station's assertions.
-- Preserve all immutable source facts and canonical identities. Unresolved
-- assertions do not participate until the owned publisher binds their UUID.
CREATE INDEX registry_assertions_station_idx
    ON registry_assertions (station_id) WHERE station_id IS NOT NULL;
