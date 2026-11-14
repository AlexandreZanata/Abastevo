-- name: InsertAccessDetail :exec
INSERT INTO org_sensitive_access_events (org_id, operation_id, category, stage, membership_id, access_id)
VALUES (CAST(sqlc.arg('org_id')::text AS uuid), CAST(sqlc.arg('operation_id')::text AS uuid), @category, @stage,
    CAST(sqlc.arg('membership_id')::text AS uuid), CAST(sqlc.arg('access_id')::text AS uuid));
