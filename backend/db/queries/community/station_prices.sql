-- Public standard station keys, independent of ANP coverage. One bounded batch.
-- name: StationPrices :many
SELECT p.* FROM community_current_prices p
WHERE p.station_id = @station_id AND (@fuel::text = '' OR p.fuel_product = @fuel)
 AND p.condition_kind = 'STANDARD' AND p.qualifier_key = 'STANDARD'
 AND (p.representative_observation_id IS NULL OR EXISTS (
  SELECT 1 FROM community_observations visible WHERE visible.id=p.representative_observation_id
   AND visible.hidden_at IS NULL AND visible.deleted_at IS NULL
   AND COALESCE((SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=visible.id ORDER BY d.sequence DESC LIMIT 1),'') <> 'REJECTED'
 ))
ORDER BY p.fuel_product, p.unit;
