-- Full city projection population; never derived from a feed page or raw votes.
-- Exact integer money and one STANDARD key per station/product/unit.
-- name: CitySummary :many
SELECT p.fuel_product, p.unit,
 ROUND(AVG(p.amount_milli_brl))::bigint AS average_milli_brl,
 MIN(p.amount_milli_brl)::bigint AS minimum_milli_brl,
 MAX(p.amount_milli_brl)::bigint AS maximum_milli_brl,
 COUNT(*)::bigint AS station_count,
 MAX(p.anchor_received_at)::timestamptz AS updated_at,
 MIN(p.expires_at)::timestamptz AS expires_at
FROM directory_stations s JOIN community_current_prices p ON p.station_id=s.id
WHERE s.state = @state AND s.municipality_code = @municipality_code AND s.status='active'
 AND p.condition_kind='STANDARD' AND p.qualifier_key='STANDARD'
 AND p.unit=CASE p.fuel_product WHEN 'CNG' THEN 'M3' WHEN 'LPG_P13' THEN 'KG_13' ELSE 'L' END
 AND p.availability='AVAILABLE' AND p.amount_milli_brl IS NOT NULL
 AND p.anchor_received_at IS NOT NULL AND p.anchor_received_at <= @now AND p.expires_at > @now
 AND (p.representative_observation_id IS NULL OR EXISTS (
  SELECT 1 FROM community_observations o WHERE o.id=p.representative_observation_id AND o.hidden_at IS NULL AND o.deleted_at IS NULL
 ))
GROUP BY p.fuel_product,p.unit
ORDER BY p.fuel_product,p.unit;
