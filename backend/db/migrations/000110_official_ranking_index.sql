-- 000110: MEX-28 indexed ANP weekly price ranking. Additive only.
-- One latest observation per station for a published revision/product/unit is
-- resolved from this index; existing rows, revisions and pointers are unchanged.
CREATE INDEX official_station_prices_ranking_idx
    ON official_station_prices
    (revision_id, fuel_product, unit, station_id, collected_on DESC, id)
    INCLUDE (amount_milli_brl);
