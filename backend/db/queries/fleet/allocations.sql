-- name: AllocationAt :one
-- EMP-FLEET-005 snapshot port: one statement reads both histories at one
-- microsecond instant, half-open; a missing period yields empty identifiers.
SELECT a.id::text AS asset_id,
 COALESCE(u.unit_id::text,'')::text AS unit_id,u.valid_from AS unit_from,u.valid_to AS unit_to,
 COALESCE(c.cost_centre_id::text,'')::text AS cost_centre_id,c.valid_from AS centre_from,c.valid_to AS centre_to
FROM org_assets a
LEFT JOIN org_asset_unit_periods u ON u.org_id=a.org_id AND u.asset_id=a.id AND u.valid_from<=sqlc.arg(at)::timestamptz AND(u.valid_to IS NULL OR sqlc.arg(at)::timestamptz<u.valid_to)
LEFT JOIN org_asset_cost_centre_periods c ON c.org_id=a.org_id AND c.asset_id=a.id AND c.valid_from<=sqlc.arg(at)::timestamptz AND(c.valid_to IS NULL OR sqlc.arg(at)::timestamptz<c.valid_to)
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid);

-- name: ListAssetUnitPeriods :many
SELECT unit_id::text AS target_id,valid_from,valid_to,recorded_at FROM org_asset_unit_periods
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR valid_from>sqlc.narg(after_from)::timestamptz)
ORDER BY valid_from LIMIT sqlc.arg(row_limit)::int;

-- name: ListAssetCostCentrePeriods :many
SELECT cost_centre_id::text AS target_id,valid_from,valid_to,recorded_at FROM org_asset_cost_centre_periods
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR valid_from>sqlc.narg(after_from)::timestamptz)
ORDER BY valid_from LIMIT sqlc.arg(row_limit)::int;
