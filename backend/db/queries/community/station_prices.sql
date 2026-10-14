-- Public standard station keys, independent of ANP coverage. One bounded batch.
-- name: StationPrices :many
SELECT p.* FROM community_current_prices p
WHERE p.station_id = @station_id AND (@fuel::text = '' OR p.fuel_product = @fuel)
 AND p.condition_kind = 'STANDARD' AND p.qualifier_key = 'STANDARD'
ORDER BY p.fuel_product, p.unit;
