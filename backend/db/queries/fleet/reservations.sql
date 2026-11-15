-- name: ReservationTxTime :one
-- The transaction instant the 217 guards compare with (B-BR-EMP-296).
SELECT transaction_timestamp()::timestamptz AS at;

-- name: ReservationCommandReceipt :one
SELECT actor_id::text AS actor_id,asset_id::text AS asset_id,kind,request_hash,target_id::text AS target_id,result_state
FROM org_asset_reservation_commands WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertReservationCommand :exec
INSERT INTO org_asset_reservation_commands(org_id,operation_id,actor_id,asset_id,kind,request_hash,target_id,result_state)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),
 sqlc.arg(kind)::text,sqlc.arg(request_hash)::text,CAST(sqlc.arg(target_id)::text AS uuid),sqlc.arg(result_state)::text);

-- name: InsertReservation :exec
-- B-BR-EMP-294: the row guard admits the asset and employee under the root
-- lock and stamps the request (and confirmation) instants.
INSERT INTO org_asset_reservations(org_id,id,asset_id,employee_id,purpose,starts_at,ends_at,no_show_tolerance,state,revision,operation_id,requested_by,confirm_operation_id,confirmed_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),NULLIF(sqlc.arg(employee_id)::text,'')::uuid,
 sqlc.arg(purpose)::text,sqlc.arg(starts_at)::timestamptz,sqlc.arg(ends_at)::timestamptz,sqlc.arg(no_show_tolerance)::int,sqlc.arg(state)::text,1,
 CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(requested_by)::text AS uuid),NULLIF(sqlc.arg(confirm_operation_id)::text,'')::uuid,NULLIF(sqlc.arg(confirmed_by)::text,'')::uuid);

-- name: ReservationForUpdate :one
SELECT id::text AS id,asset_id::text AS asset_id,requested_by::text AS requested_by,state,revision,starts_at,ends_at,no_show_tolerance,(confirmed_at IS NOT NULL)::boolean AS confirmed,ended_at
FROM org_asset_reservations WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid)
FOR UPDATE;

-- name: DueAssetReservations :many
-- B-BR-EMP-296: the reservations of the asset whose time-driven outcome is due
-- at the instant, locked in identifier order for their settlement.
SELECT id::text AS id FROM org_asset_reservations
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(state='REQUESTED' AND starts_at<=sqlc.arg(at)::timestamptz OR state='CONFIRMED' AND org_asset_reservation_deadline(starts_at,ends_at,no_show_tolerance)<=sqlc.arg(at)::timestamptz)
ORDER BY id FOR UPDATE;

-- name: ConfirmReservationRow :execrows
UPDATE org_asset_reservations SET state='CONFIRMED',revision=revision+1,confirm_operation_id=CAST(sqlc.arg(confirm_operation_id)::text AS uuid),confirmed_by=CAST(sqlc.arg(confirmed_by)::text AS uuid)
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state='REQUESTED' AND revision=sqlc.arg(revision)::bigint;

-- name: CancelReservationRow :execrows
UPDATE org_asset_reservations SET state='CANCELLED',revision=revision+1,end_cause='CANCELLED',end_operation_id=CAST(sqlc.arg(end_operation_id)::text AS uuid),ended_by=CAST(sqlc.arg(ended_by)::text AS uuid),end_reason=sqlc.arg(end_reason)::text
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state IN('REQUESTED','CONFIRMED') AND revision=sqlc.arg(revision)::bigint;

-- name: SettleReservationRow :execrows
-- Compare-and-set of one time-driven outcome: a repeat changes nothing.
UPDATE org_asset_reservations SET state=sqlc.arg(state)::text,revision=revision+1,end_cause='TIME'
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state=sqlc.arg(from_state)::text AND revision=sqlc.arg(revision)::bigint;

-- name: ConfirmedConflict :one
-- B-BR-EMP-295: the earliest CONFIRMED (or FULFILLED) window of another
-- reservation of the asset overlapping the window; only the window leaves.
SELECT starts_at,ends_at FROM org_asset_reservations
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND id<>CAST(sqlc.arg(id)::text AS uuid)
 AND state IN('CONFIRMED','FULFILLED') AND tstzrange(starts_at,ends_at,'[)')&&tstzrange(sqlc.arg(starts_at)::timestamptz,sqlc.arg(ends_at)::timestamptz,'[)')
ORDER BY starts_at,id LIMIT 1;

-- name: BlockConflict :one
SELECT valid_from,valid_to FROM org_asset_blocks
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND tstzrange(valid_from,valid_to,'[)')&&tstzrange(sqlc.arg(starts_at)::timestamptz,sqlc.arg(ends_at)::timestamptz,'[)')
ORDER BY valid_from,id LIMIT 1;

-- name: InsertBlock :exec
INSERT INTO org_asset_blocks(org_id,id,asset_id,reason_kind,note,valid_from,valid_to,operation_id,blocked_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),sqlc.arg(reason_kind)::text,sqlc.arg(note)::text,
 sqlc.arg(valid_from)::timestamptz,sqlc.narg(valid_to)::timestamptz,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(blocked_by)::text AS uuid));

-- name: BlockForRemoval :one
SELECT id::text AS id,valid_from,valid_to,(removed_at IS NOT NULL)::boolean AS removed FROM org_asset_blocks
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid)
FOR UPDATE;

-- name: RemoveBlockRow :execrows
UPDATE org_asset_blocks SET valid_to=sqlc.arg(valid_to)::timestamptz,remove_operation_id=CAST(sqlc.arg(remove_operation_id)::text AS uuid),removed_by=CAST(sqlc.arg(removed_by)::text AS uuid),removal_reason=sqlc.arg(removal_reason)::text
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND removed_at IS NULL;

-- name: ListAssetReservations :many
SELECT id::text AS id,asset_id::text AS asset_id,COALESCE(employee_id::text,'')::text AS employee_id,purpose,starts_at,ends_at,no_show_tolerance,state,requested_at,confirmed_at,end_cause,ended_at,end_reason
FROM org_asset_reservations
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR(starts_at,id)>(sqlc.narg(after_from)::timestamptz,NULLIF(sqlc.arg(after_id)::text,'')::uuid))
ORDER BY starts_at,id LIMIT sqlc.arg(row_limit)::int;

-- name: ListAssetBlocks :many
SELECT id::text AS id,asset_id::text AS asset_id,reason_kind,note,valid_from,valid_to,recorded_at,removed_at,removal_reason
FROM org_asset_blocks
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR(valid_from,id)>(sqlc.narg(after_from)::timestamptz,NULLIF(sqlc.arg(after_id)::text,'')::uuid))
ORDER BY valid_from,id LIMIT sqlc.arg(row_limit)::int;

-- name: AvailabilityBlocks :many
-- B-BR-EMP-299: blocks whose period contains the instant.
SELECT id::text AS id,reason_kind,valid_from,valid_to FROM org_asset_blocks
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND valid_from<=sqlc.arg(at)::timestamptz AND(valid_to IS NULL OR sqlc.arg(at)::timestamptz<valid_to)
ORDER BY valid_from,id;

-- name: AvailabilityReservations :many
-- Reservations that were ever confirmed and whose window contains the
-- instant; the domain decides whether each held the asset then.
SELECT id::text AS id,COALESCE(employee_id::text,'')::text AS employee_id,state,starts_at,ends_at,no_show_tolerance,(confirmed_at IS NOT NULL)::boolean AS confirmed,ended_at
FROM org_asset_reservations
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND confirmed_at IS NOT NULL
 AND starts_at<=sqlc.arg(at)::timestamptz AND sqlc.arg(at)::timestamptz<ends_at
ORDER BY starts_at,id;

-- name: MemberEmployee :one
-- The employee record of a membership whose period contains the instant.
SELECT employee_id::text AS employee_id FROM org_memberships
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND started_at IS NOT NULL AND started_at<=sqlc.arg(at)::timestamptz AND(ended_at IS NULL OR sqlc.arg(at)::timestamptz<ended_at);
