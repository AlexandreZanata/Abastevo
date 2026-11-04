-- Full city projection population; never derived from a feed page or raw votes.
-- Exact integer money and one STANDARD key per station/product/unit.
-- name: CitySummary :many
WITH eligible_prices AS (
 SELECT p.fuel_product, p.unit, p.amount_milli_brl, p.confirmation_count,
  p.anchor_received_at, p.expires_at,
  MIN(p.amount_milli_brl) OVER fuel_window AS minimum_price,
  MAX(p.amount_milli_brl) OVER fuel_window AS maximum_price
 FROM directory_stations s JOIN community_current_prices p ON p.station_id=s.id
WHERE s.state = @state AND s.municipality_code = @municipality_code AND s.status='active'
 AND p.condition_kind='STANDARD' AND p.qualifier_key='STANDARD'
 AND p.unit=CASE p.fuel_product WHEN 'CNG' THEN 'M3' WHEN 'LPG_P13' THEN 'KG_13' ELSE 'L' END
 AND p.availability='AVAILABLE' AND p.amount_milli_brl IS NOT NULL
 AND p.anchor_received_at IS NOT NULL AND p.anchor_received_at <= @now AND p.expires_at > @now
 AND (p.representative_observation_id IS NULL OR EXISTS (
  SELECT 1 FROM community_observations o WHERE o.id=p.representative_observation_id AND o.hidden_at IS NULL AND o.deleted_at IS NULL
   AND COALESCE((SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=o.id ORDER BY d.sequence DESC LIMIT 1),'') <> 'REJECTED'
 ))
 WINDOW fuel_window AS (PARTITION BY p.fuel_product, p.unit)
)
SELECT fuel_product, unit,
 ROUND(AVG(amount_milli_brl))::bigint AS average_milli_brl,
 MIN(minimum_price)::bigint AS minimum_milli_brl,
 MAX(maximum_price)::bigint AS maximum_milli_brl,
 COUNT(*)::bigint AS station_count,
 SUM(confirmation_count)::bigint AS total_confirmations,
 SUM(confirmation_count) FILTER (WHERE amount_milli_brl = minimum_price)::bigint AS minimum_confirmations,
 SUM(confirmation_count) FILTER (WHERE amount_milli_brl = maximum_price)::bigint AS maximum_confirmations,
 MAX(anchor_received_at)::timestamptz AS updated_at,
 MIN(expires_at)::timestamptz AS expires_at
FROM eligible_prices
GROUP BY fuel_product,unit
ORDER BY fuel_product,unit;
