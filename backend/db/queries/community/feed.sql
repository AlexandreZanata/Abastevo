-- Owned public community read: the explicitly declared directory join reads
-- only canonical public identity/city fields, never another module's private data.
-- name: CityFeed :many
SELECT p.station_id, s.display_name, p.fuel_product, p.unit,
 p.amount_milli_brl, p.confidence, p.independent_supporters,
 p.confirmation_count, p.anchor_received_at, p.expires_at, p.projection_version
FROM directory_stations s
JOIN community_current_prices p ON p.station_id = s.id
WHERE s.state = @state AND s.municipality_code = @municipality_code AND s.status = 'active'
 AND p.fuel_product = @fuel_product AND p.unit = @unit
 AND p.condition_kind = 'STANDARD' AND p.qualifier_key = 'STANDARD'
 AND p.availability = 'AVAILABLE' AND p.amount_milli_brl IS NOT NULL
 AND p.anchor_received_at IS NOT NULL AND p.anchor_received_at <= @now
 AND p.expires_at > @now
 AND (NOT @has_cursor::boolean OR
  (@sort_order::text = 'recent' AND (p.anchor_received_at < @after_time OR
    (p.anchor_received_at = @after_time AND p.station_id::text > @after_id::text))) OR
  (@sort_order::text = 'cheapest' AND (p.amount_milli_brl > @after_amount::bigint OR
    (p.amount_milli_brl = @after_amount::bigint AND p.station_id::text > @after_id::text))))
ORDER BY CASE WHEN @sort_order::text = 'cheapest' THEN p.amount_milli_brl END ASC,
 CASE WHEN @sort_order::text = 'recent' THEN p.anchor_received_at END DESC, p.station_id ASC
LIMIT @limit_plus_one::int;
