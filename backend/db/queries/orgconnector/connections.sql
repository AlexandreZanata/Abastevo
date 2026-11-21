-- name: ReadConnectorApproval :one
SELECT id::text AS id,revision,kind,provider_code,
       COALESCE(resource_id::text,'')::text AS resource_id,COALESCE(credential_ref::text,'')::text AS credential_ref,
       maximum_bytes,maximum_items,decision,ready_at
FROM org_connector_approvals
WHERE org_id=sqlc.arg('org_id')::text::uuid AND id=sqlc.arg('id')::text::uuid
ORDER BY revision DESC LIMIT 1;

-- name: GetConnection :one
SELECT c.id::text AS id,c.approval_id::text AS approval_id,c.approval_revision,c.label,c.state,c.revision,c.changed_at,
       a.kind,a.provider_code,
       (a.decision='APPROVED' AND NOT EXISTS (
           SELECT 1 FROM org_connector_approvals newer
           WHERE newer.org_id=a.org_id AND newer.id=a.id AND newer.revision>a.revision
       ))::boolean AS approval_current
FROM org_connections c
JOIN org_connector_approvals a ON (a.org_id,a.id,a.revision)=(c.org_id,c.approval_id,c.approval_revision)
WHERE c.org_id=sqlc.arg('org_id')::text::uuid AND c.id=sqlc.arg('id')::text::uuid
FOR SHARE OF c;

-- name: ListConnections :many
SELECT c.id::text AS id,c.label,c.state,c.revision,c.changed_at,a.kind,a.provider_code,
       (a.decision='APPROVED' AND NOT EXISTS (
           SELECT 1 FROM org_connector_approvals newer
           WHERE newer.org_id=a.org_id AND newer.id=a.id AND newer.revision>a.revision
       ))::boolean AS approval_current
FROM org_connections c
JOIN org_connector_approvals a ON (a.org_id,a.id,a.revision)=(c.org_id,c.approval_id,c.approval_revision)
WHERE c.org_id=sqlc.arg('org_id')::text::uuid
  AND (sqlc.arg('after_id')::text='' OR c.id>NULLIF(sqlc.arg('after_id')::text,'')::uuid)
ORDER BY c.id LIMIT sqlc.arg('row_limit')::integer;

-- name: ConfigureConnection :execrows
INSERT INTO org_connections (org_id,id,approval_id,approval_revision,label,state,revision,changed_at)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('id')::text::uuid,sqlc.arg('approval_id')::text::uuid,
       sqlc.arg('approval_revision')::bigint,sqlc.arg('label')::text,'CONFIGURED',1,sqlc.arg('changed_at')::timestamptz)
ON CONFLICT DO NOTHING;

-- name: ChangeConnectionState :execrows
UPDATE org_connections SET state=sqlc.arg('state')::text,revision=revision+1,
    changed_at=sqlc.arg('changed_at')::timestamptz
WHERE org_id=sqlc.arg('org_id')::text::uuid AND id=sqlc.arg('id')::text::uuid
  AND revision=sqlc.arg('expected_revision')::bigint AND state<>sqlc.arg('state')::text;

-- name: ReadConnectionClock :one
SELECT pg_catalog.clock_timestamp()::timestamptz AS observed_at;

-- name: LockConnectorOrganization :one
SELECT state::text FROM organizations
WHERE org_id=sqlc.arg('org_id')::text::uuid AND has_snapshot
FOR SHARE;

-- name: LockConnectorApproval :exec
SELECT pg_catalog.pg_advisory_xact_lock_shared(pg_catalog.hashtextextended(
    'org-connector-approval:' || sqlc.arg('org_id')::text || ':' || sqlc.arg('id')::text,0
));
