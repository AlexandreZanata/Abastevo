-- Owned public community read: the explicitly declared directory join reads
-- only canonical public identity/city fields, never another module's private data.
-- Best/worst orders rank by the community star average of the same
-- station+fuel (feedback_rating_stats); unrated rows sort last in both.
-- name: CityFeed :many
SELECT p.station_id, s.display_name, p.fuel_product, p.unit,
 p.amount_milli_brl, p.confidence, p.independent_supporters,
 p.confirmation_count, p.anchor_received_at, p.expires_at, p.projection_version,
 COALESCE(rs.ratings_count, 0)::bigint AS ratings_count,
 COALESCE(rs.stars_sum, 0)::bigint AS stars_sum,
 o.id AS contribution_observation_id,
 COALESCE(d.sequence,0)::bigint AS contribution_decision_sequence,
 COALESCE(d.to_state,'')::text AS contribution_state
FROM directory_stations s
JOIN community_current_prices p ON p.station_id = s.id
LEFT JOIN feedback_rating_stats rs ON rs.station_id = p.station_id AND rs.product = p.fuel_product
LEFT JOIN community_observations o ON o.id = p.representative_observation_id AND o.hidden_at IS NULL AND o.deleted_at IS NULL
LEFT JOIN LATERAL (
 SELECT sequence,to_state FROM community_observation_decisions WHERE observation_id=o.id ORDER BY sequence DESC LIMIT 1
) d ON true
WHERE s.state = @state AND s.municipality_code = @municipality_code AND s.status = 'active'
 AND p.fuel_product = @fuel_product AND p.unit = @unit
 AND p.condition_kind = 'STANDARD' AND p.qualifier_key = 'STANDARD'
 AND p.availability = 'AVAILABLE' AND p.amount_milli_brl IS NOT NULL
 AND p.anchor_received_at IS NOT NULL AND p.anchor_received_at <= @now
 AND p.expires_at > @now
 AND (p.representative_observation_id IS NULL OR EXISTS (
  SELECT 1 FROM community_observations visible WHERE visible.id=p.representative_observation_id
   AND visible.hidden_at IS NULL AND visible.deleted_at IS NULL
   AND COALESCE((SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=visible.id ORDER BY d.sequence DESC LIMIT 1),'') <> 'REJECTED'
 ))
 AND (NOT @has_cursor::boolean OR
  (@sort_order::text = 'recent' AND (p.anchor_received_at < @after_time OR
    (p.anchor_received_at = @after_time AND p.station_id::text > @after_id::text))) OR
  (@sort_order::text = 'cheapest' AND (p.amount_milli_brl > @after_amount::bigint OR
    (p.amount_milli_brl = @after_amount::bigint AND p.station_id::text > @after_id::text))) OR
  (@sort_order::text = 'best' AND (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) < NULLIF(@after_avg::double precision, -1) OR
    (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) = NULLIF(@after_avg::double precision, -1) AND p.station_id::text > @after_id::text) OR
    (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) IS NULL AND (NULLIF(@after_avg::double precision, -1) IS NULL AND p.station_id::text > @after_id::text OR NULLIF(@after_avg::double precision, -1) IS NOT NULL)))) OR
  (@sort_order::text = 'worst' AND (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) > NULLIF(@after_avg::double precision, -1) OR
    (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) = NULLIF(@after_avg::double precision, -1) AND p.station_id::text > @after_id::text) OR
    (((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) IS NULL AND (NULLIF(@after_avg::double precision, -1) IS NULL AND p.station_id::text > @after_id::text OR NULLIF(@after_avg::double precision, -1) IS NOT NULL)))))
ORDER BY CASE WHEN @sort_order::text = 'cheapest' THEN p.amount_milli_brl END ASC,
 CASE WHEN @sort_order::text = 'recent' THEN p.anchor_received_at END DESC,
 CASE WHEN @sort_order::text = 'best' THEN ((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) END DESC NULLS LAST,
 CASE WHEN @sort_order::text = 'worst' THEN ((rs.stars_sum::double precision / NULLIF(rs.ratings_count, 0))::double precision) END ASC NULLS LAST,
 p.station_id ASC
LIMIT @limit_plus_one::int;
