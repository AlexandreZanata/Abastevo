-- name: AttachmentCommandReceipt :one
SELECT actor_id::text AS actor_id,asset_id::text AS asset_id,kind,request_hash,attachment_id::text AS attachment_id
FROM org_asset_attachment_commands WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertAttachmentCommand :exec
INSERT INTO org_asset_attachment_commands(org_id,operation_id,actor_id,asset_id,kind,request_hash,attachment_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),
 sqlc.arg(kind)::text,sqlc.arg(request_hash)::text,CAST(sqlc.arg(attachment_id)::text AS uuid));

-- name: InsertAttachment :exec
-- B-BR-EMP-325: the row guard checks both assets' kinds and states under the
-- root lock and stamps the recording instant.
INSERT INTO org_asset_implement_attachments(org_id,id,implement_id,powered_asset_id,valid_from,valid_to,operation_id,attached_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(implement_id)::text AS uuid),CAST(sqlc.arg(powered_asset_id)::text AS uuid),
 sqlc.arg(valid_from)::timestamptz,sqlc.narg(valid_to)::timestamptz,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(attached_by)::text AS uuid));

-- name: AttachmentAssetKind :one
SELECT kind,state FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: AttachmentForDetach :one
-- The attachment where the path asset is the implement or the powered asset.
SELECT id::text AS id,implement_id::text AS implement_id,powered_asset_id::text AS powered_asset_id,valid_from,valid_to,(detached_at IS NOT NULL)::boolean AS detached
FROM org_asset_implement_attachments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid)
 AND(implement_id=CAST(sqlc.arg(asset_id)::text AS uuid) OR powered_asset_id=CAST(sqlc.arg(asset_id)::text AS uuid))
FOR UPDATE;

-- name: DetachAttachmentRow :execrows
UPDATE org_asset_implement_attachments SET valid_to=sqlc.arg(valid_to)::timestamptz,detach_operation_id=CAST(sqlc.arg(detach_operation_id)::text AS uuid),
 detached_by=CAST(sqlc.arg(detached_by)::text AS uuid),detach_reason=sqlc.arg(detach_reason)::text
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND detached_at IS NULL;

-- name: ListAssetAttachments :many
SELECT id::text AS id,implement_id::text AS implement_id,powered_asset_id::text AS powered_asset_id,valid_from,valid_to,recorded_at,detached_at,detach_reason
FROM org_asset_implement_attachments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND(implement_id=CAST(sqlc.arg(asset_id)::text AS uuid) OR powered_asset_id=CAST(sqlc.arg(asset_id)::text AS uuid))
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR(valid_from,id)>(sqlc.narg(after_from)::timestamptz,NULLIF(sqlc.arg(after_id)::text,'')::uuid))
ORDER BY valid_from,id LIMIT sqlc.arg(row_limit)::int;

-- name: ImplementsAttachedAt :many
-- BR-FLEET-033: the implements attached to the powered asset at the instant.
SELECT implement_id::text AS implement_id FROM org_asset_implement_attachments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND powered_asset_id=CAST(sqlc.arg(powered_asset_id)::text AS uuid)
 AND valid_from<=sqlc.arg(at)::timestamptz AND(valid_to IS NULL OR sqlc.arg(at)::timestamptz<valid_to)
ORDER BY implement_id;

-- name: PoweredAssetAt :many
-- The powered asset an implement is attached to at the instant (at most one
-- by the exclusion constraint).
SELECT powered_asset_id::text AS powered_asset_id FROM org_asset_implement_attachments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND implement_id=CAST(sqlc.arg(implement_id)::text AS uuid)
 AND valid_from<=sqlc.arg(at)::timestamptz AND(valid_to IS NULL OR sqlc.arg(at)::timestamptz<valid_to);
