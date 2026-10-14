-- Owned community facts: trust changes affect authored anchors AND independent
-- confirmations. Stable keyset pages retain every affected historical key.
-- name: AffectedContributorKeys :many
WITH affected AS (
 SELECT o.station_id, o.fuel_product, o.unit, o.condition_kind, o.qualifier_key
 FROM community_observations o WHERE o.contributor_ref = @contributor_ref
 UNION
 SELECT o.station_id, o.fuel_product, o.unit, o.condition_kind, o.qualifier_key
 FROM community_confirmations c JOIN community_observations o ON o.id=c.observation_id
 WHERE c.contributor_ref = @contributor_ref
)
SELECT station_id, fuel_product, unit, condition_kind, qualifier_key FROM affected
WHERE NOT @has_cursor::boolean OR
 (station_id::text,fuel_product,unit,condition_kind,qualifier_key) >
 (@after_station_id::text,@after_product::text,@after_unit::text,@after_condition::text,@after_qualifier::text)
ORDER BY station_id,fuel_product,unit,condition_kind,qualifier_key
LIMIT @page_limit::int;
