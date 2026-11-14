-- name: TransitionAssetLifecycle :execrows
-- EMP-FLEET-003: lifecycle columns only; the profile and unit stay unchanged.
UPDATE org_assets SET state=sqlc.arg(state)::text,revision=revision+1,
 acquired_at=sqlc.arg(acquired_at)::timestamptz,acquired_submicro=sqlc.arg(acquired_submicro)::smallint,
 disposal_kind=NULLIF(sqlc.arg(disposal_kind)::text,''),disposed_at=sqlc.arg(disposed_at)::timestamptz,disposed_submicro=sqlc.arg(disposed_submicro)::smallint,
 disposal_reason=NULLIF(sqlc.arg(disposal_reason)::text,'')
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND revision=sqlc.arg(expected_revision)::bigint;

-- name: InsertAssetLifecycleEvent :exec
INSERT INTO org_asset_lifecycle_events(org_id,asset_id,revision,operation_id,actor_id,command,from_state,to_state,reason,disposal_kind,disposed_at,disposed_submicro)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),sqlc.arg(revision)::bigint,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),
 sqlc.arg(command)::text,sqlc.arg(from_state)::text,sqlc.arg(to_state)::text,sqlc.arg(reason)::text,NULLIF(sqlc.arg(disposal_kind)::text,''),sqlc.arg(disposed_at)::timestamptz,sqlc.arg(disposed_submicro)::smallint);

-- name: ListAssetLifecycleEvents :many
SELECT revision,command,from_state,to_state,reason,COALESCE(disposal_kind,'')::text AS disposal_kind,disposed_at,disposed_submicro,recorded_at
FROM org_asset_lifecycle_events WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND revision>sqlc.arg(after_revision)::bigint
ORDER BY revision LIMIT sqlc.arg(row_limit)::int;

-- name: AssetLifecycleTarget :one
SELECT id::text AS id FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MeterAssetDisposal :one
-- Taken after the organization root lock, in the intake transaction.
SELECT disposed_at,disposed_submicro FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;
