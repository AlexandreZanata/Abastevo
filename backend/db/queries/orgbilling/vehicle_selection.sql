-- name: ReadVehicleDowngrade :one
SELECT ceiling::bigint AS ceiling,term_start::timestamptz AS term_start,previous_start::timestamptz AS previous_start
FROM org_billing_vehicle_downgrade(sqlc.arg('org_id')::text::uuid,sqlc.arg('subscription_id')::text::uuid,
    sqlc.arg('subscription_revision')::bigint,sqlc.arg('evaluated_at')::timestamptz);

-- name: PlanSelectionActiveVehicles :many
SELECT id::text AS id,revision FROM org_assets
WHERE org_id=sqlc.arg('org_id')::text::uuid AND kind='VEHICLE' AND state='ACTIVE'
ORDER BY id;

-- name: ReadVehicleSelection :one
SELECT kept::text[] AS kept,archived::text[] AS archived FROM org_billing_vehicle_plan_selections
WHERE (org_id,operation_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid);

-- name: WriteVehicleSelection :execrows
INSERT INTO org_billing_vehicle_plan_selections(org_id,operation_id,subscription_id,subscription_revision,kept)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('subscription_id')::text::uuid,
    sqlc.arg('subscription_revision')::bigint,sqlc.arg('kept')::text[]::uuid[])
ON CONFLICT DO NOTHING;

-- name: WriteVehiclePlanArchive :exec
INSERT INTO org_billing_vehicle_plan_archives(org_id,selection_id,asset_id,asset_revision)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('selection_id')::text::uuid,sqlc.arg('asset_id')::text::uuid,sqlc.arg('asset_revision')::bigint);

-- name: CountPlanSelectionActiveVehicles :one
SELECT count(*)::bigint FROM org_assets
WHERE org_id=sqlc.arg('org_id')::text::uuid AND kind='VEHICLE' AND state='ACTIVE';
