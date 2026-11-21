-- name: ReadWebhookEndpoint :one
SELECT endpoint FROM org_webhook_endpoints
WHERE org_id=sqlc.arg('org_id')::text::uuid AND approval_id=sqlc.arg('approval_id')::text::uuid
  AND approval_revision=sqlc.arg('approval_revision')::bigint;

-- name: ListWebhookDestinations :many
SELECT c.id::text AS connection_id,c.revision
FROM org_asset_lifecycle_events l
JOIN org_assets asset ON (asset.org_id,asset.id)=(l.org_id,l.asset_id)
JOIN org_connector_approvals a ON (a.org_id,a.resource_id)=(l.org_id,l.asset_id)
JOIN org_connections c ON (c.org_id,c.approval_id,c.approval_revision)=(a.org_id,a.id,a.revision)
JOIN org_webhook_endpoints e ON (e.org_id,e.approval_id,e.approval_revision)=(a.org_id,a.id,a.revision)
WHERE l.org_id=sqlc.arg('org_id')::text::uuid AND l.operation_id=sqlc.arg('operation_id')::text::uuid
  AND l.command IN ('DEACTIVATE','REACTIVATE') AND asset.kind='VEHICLE'
  AND a.kind='OUTBOUND_WEBHOOK' AND a.decision='APPROVED' AND a.ready_at<=l.recorded_at
  AND c.state='ACTIVE' AND c.changed_at<=l.recorded_at
  AND NOT EXISTS (SELECT 1 FROM org_connector_approvals n WHERE (n.org_id,n.id)=(a.org_id,a.id) AND n.revision>a.revision)
ORDER BY c.id;

-- name: InsertWebhookDelivery :exec
INSERT INTO org_webhook_deliveries (org_id,id,connection_id,operation_id,job_id,connection_revision)
VALUES (sqlc.arg('org_id')::text::uuid,sqlc.arg('id')::text::uuid,sqlc.arg('connection_id')::text::uuid,
        sqlc.arg('operation_id')::text::uuid,sqlc.arg('job_id')::text::uuid,sqlc.arg('connection_revision')::bigint);

-- name: ClaimWebhookDelivery :one
UPDATE org_webhook_deliveries d SET attempts=d.attempts+1,outcome='UNKNOWN',last_lease=j.lease_token
FROM job_queue j,org_asset_lifecycle_events l
WHERE d.org_id=sqlc.arg('org_id')::text::uuid AND d.id=sqlc.arg('id')::text::uuid
  AND d.job_id=sqlc.arg('job_id')::text::uuid AND j.id=d.job_id AND j.kind='org_connector.fleet_webhook'
  AND j.status='leased' AND j.lease_token=sqlc.arg('lease_token')::bigint AND j.lease_token>d.last_lease
  AND j.lease_expires_at>clock_timestamp()+interval '6 seconds'
  AND (l.org_id,l.operation_id)=(d.org_id,d.operation_id)
  AND d.attempts<3 AND d.outcome IN ('PENDING','RETRY','UNKNOWN')
RETURNING d.connection_id::text AS connection_id,d.connection_revision,d.attempts,
          l.asset_id::text AS asset_id,l.command,l.recorded_at,j.lease_expires_at;

-- name: FinishWebhookDelivery :execrows
UPDATE org_webhook_deliveries SET outcome=sqlc.arg('outcome')::text
WHERE org_id=sqlc.arg('org_id')::text::uuid AND id=sqlc.arg('id')::text::uuid
  AND last_lease=sqlc.arg('lease_token')::bigint AND outcome='UNKNOWN';

-- name: WebhookDeliveryComplete :one
SELECT attempts=3 OR outcome IN ('DELIVERED','FAILED','SUPPRESSED') FROM org_webhook_deliveries
WHERE org_id=sqlc.arg('org_id')::text::uuid AND id=sqlc.arg('id')::text::uuid AND job_id=sqlc.arg('job_id')::text::uuid;
