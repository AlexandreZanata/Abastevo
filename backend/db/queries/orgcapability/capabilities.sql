-- name: GetState :one
SELECT enabled, revision FROM org_capabilities
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid) AND capability = @capability;

-- name: LockStateForWrite :one
SELECT enabled, revision FROM org_capabilities
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid) AND capability = @capability
FOR SHARE;

-- name: CreateState :execrows
INSERT INTO org_capabilities (org_id, capability, enabled)
VALUES (CAST(sqlc.arg('org_id')::text AS uuid), @capability, @enabled)
ON CONFLICT (org_id, capability) DO NOTHING;

-- name: AdvanceState :execrows
UPDATE org_capabilities SET enabled = @enabled, revision = revision + 1, updated_at = clock_timestamp()
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid) AND capability = @capability
  AND revision = @expected_revision;

-- name: LockEntitlementRead :exec
SELECT pg_catalog.pg_advisory_xact_lock_shared(
    pg_catalog.hashtextextended('organization-entitlement:' || sqlc.arg('org_id')::text, 0));

-- name: LockEntitlementWrite :exec
SELECT pg_catalog.pg_advisory_xact_lock(
    pg_catalog.hashtextextended('organization-entitlement:' || sqlc.arg('org_id')::text, 0));
