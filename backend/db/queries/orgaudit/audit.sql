-- name: ReserveEvent :one
INSERT INTO org_audit_events
    (org_id, operation_id, actor_id, delegation_id, action, target_type, target_id, reason_code, request_hash)
VALUES
    (CAST(sqlc.arg('org_id')::text AS uuid), CAST(sqlc.arg('operation_id')::text AS uuid), CAST(sqlc.arg('actor_id')::text AS uuid),
     NULLIF(@delegation_id::text, '')::uuid, @action, @target_type,
     CAST(sqlc.arg('target_id')::text AS uuid), @reason_code, @request_hash)
ON CONFLICT (org_id, operation_id) DO NOTHING
RETURNING operation_id::text;

-- name: GetEvent :one
SELECT operation_id::text AS operation_id, actor_id::text AS actor_id,
    COALESCE(delegation_id::text, '')::text AS delegation_id,
    action, target_type, target_id::text AS target_id, reason_code, request_hash, recorded_at
FROM org_audit_events
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid) AND operation_id = CAST(sqlc.arg('operation_id')::text AS uuid);

-- name: ListEvents :many
SELECT operation_id::text AS operation_id, actor_id::text AS actor_id,
    COALESCE(delegation_id::text, '')::text AS delegation_id,
    action, target_type, target_id::text AS target_id, reason_code, request_hash, recorded_at
FROM org_audit_events
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid)
  AND (sqlc.narg('before_at')::timestamptz IS NULL OR
       (recorded_at, operation_id) < (sqlc.narg('before_at')::timestamptz, NULLIF(@before_id::text, '')::uuid))
ORDER BY recorded_at DESC, operation_id DESC
LIMIT @row_limit;
