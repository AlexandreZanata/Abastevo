-- name: AssignmentCommandReceipt :one
SELECT actor_id::text AS actor_id,asset_id::text AS asset_id,kind,request_hash,assignment_ids::text[] AS assignment_ids,COALESCE(replaced_id::text,'')::text AS replaced_id
FROM org_asset_assignment_commands WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertAssignmentCommand :exec
INSERT INTO org_asset_assignment_commands(org_id,operation_id,actor_id,asset_id,kind,request_hash,assignment_ids,replaced_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),
 sqlc.arg(kind)::text,sqlc.arg(request_hash)::text,CAST(sqlc.arg(assignment_ids)::text[] AS uuid[]),NULLIF(sqlc.arg(replaced_id)::text,'')::uuid);

-- name: InsertAssignment :exec
-- B-BR-EMP-243: the row guard admits the asset and employee under the root
-- lock and stamps the recording instant.
INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,valid_to,reason,operation_id,assigned_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(employee_id)::text AS uuid),
 sqlc.arg(kind)::text,sqlc.arg(valid_from)::timestamptz,sqlc.narg(valid_to)::timestamptz,sqlc.arg(reason)::text,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(assigned_by)::text AS uuid));

-- name: CoveringPrimaryForUpdate :many
-- B-BR-EMP-244: the primary whose half-open period contains the instant (at
-- most one by the exclusion constraint), locked for the replace.
SELECT id::text AS id,valid_from,valid_to FROM org_asset_assignments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND kind='PRIMARY'
 AND valid_from<=sqlc.arg(at)::timestamptz AND(valid_to IS NULL OR sqlc.arg(at)::timestamptz<valid_to)
ORDER BY id FOR UPDATE;

-- name: AssignmentForEnd :one
SELECT id::text AS id,valid_from,valid_to FROM org_asset_assignments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid)
FOR UPDATE;

-- name: EndAssignmentByCommand :execrows
UPDATE org_asset_assignments SET valid_to=sqlc.arg(valid_to)::timestamptz,end_cause=sqlc.arg(end_cause)::text,
 end_operation_id=CAST(sqlc.arg(end_operation_id)::text AS uuid),ended_by=CAST(sqlc.arg(ended_by)::text AS uuid),end_reason=sqlc.arg(end_reason)::text,source_revision=NULL
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ListAssetAssignments :many
SELECT id::text AS id,asset_id::text AS asset_id,employee_id::text AS employee_id,kind,valid_from,valid_to,reason,recorded_at,end_cause,ended_at
FROM org_asset_assignments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
 AND(sqlc.narg(after_from)::timestamptz IS NULL OR(valid_from,id)>(sqlc.narg(after_from)::timestamptz,NULLIF(sqlc.arg(after_id)::text,'')::uuid))
ORDER BY valid_from,id LIMIT sqlc.arg(row_limit)::int;

-- name: ListOwnAssignments :many
-- B-BR-EMP-246: assignments of the membership's employee record whose period
-- contains the instant while the membership period contains it too.
SELECT a.id::text AS id,a.asset_id::text AS asset_id,a.employee_id::text AS employee_id,a.kind,a.valid_from,a.valid_to,a.reason,a.recorded_at,a.end_cause,a.ended_at
FROM org_asset_assignments a JOIN org_memberships m ON(m.org_id,m.employee_id)=(a.org_id,a.employee_id)
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND m.started_at IS NOT NULL AND m.started_at<=sqlc.arg(at)::timestamptz AND(m.ended_at IS NULL OR sqlc.arg(at)::timestamptz<m.ended_at)
 AND a.valid_from<=sqlc.arg(at)::timestamptz AND(a.valid_to IS NULL OR sqlc.arg(at)::timestamptz<a.valid_to)
 AND(sqlc.arg(after_id)::text='' OR a.id>NULLIF(sqlc.arg(after_id)::text,'')::uuid)
ORDER BY a.id LIMIT sqlc.arg(row_limit)::int;

-- name: AssetAssignedAt :one
SELECT EXISTS(SELECT 1 FROM org_asset_assignments a JOIN org_memberships m ON(m.org_id,m.employee_id)=(a.org_id,a.employee_id)
 WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid)
  AND m.started_at IS NOT NULL AND m.started_at<=sqlc.arg(at)::timestamptz AND(m.ended_at IS NULL OR sqlc.arg(at)::timestamptz<m.ended_at)
  AND a.valid_from<=sqlc.arg(at)::timestamptz AND(a.valid_to IS NULL OR sqlc.arg(at)::timestamptz<a.valid_to)) AS assigned;

-- name: AssetsAssignedAt :many
SELECT a.asset_id::text AS asset_id FROM org_asset_assignments a JOIN org_memberships m ON(m.org_id,m.employee_id)=(a.org_id,a.employee_id)
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND m.started_at IS NOT NULL AND m.started_at<=sqlc.arg(at)::timestamptz AND(m.ended_at IS NULL OR sqlc.arg(at)::timestamptz<m.ended_at)
 AND a.valid_from<=sqlc.arg(at)::timestamptz AND(a.valid_to IS NULL OR sqlc.arg(at)::timestamptz<a.valid_to)
GROUP BY a.asset_id ORDER BY a.asset_id;

-- name: MemberAssetAssignmentPeriods :many
-- EMP-METER-002 (EDR-173): every non-empty retained period in which the asset
-- was assigned to the membership's employee record while the membership
-- period contained it (B-BR-EMP-246), in start order. An instant lies in one
-- of them exactly when AssetAssignedAt answers true for it. disposal_ended is
-- set only when the assignment end, ended by the asset's disposal, bounds it.
SELECT GREATEST(a.valid_from,m.started_at)::timestamptz AS period_from,
 LEAST(a.valid_to,m.ended_at)::timestamptz AS period_to,
 (a.end_cause='ASSET_DISPOSED' AND a.valid_to IS NOT NULL AND(m.ended_at IS NULL OR a.valid_to<=m.ended_at))::boolean AS disposal_ended
FROM org_asset_assignments a JOIN org_memberships m ON(m.org_id,m.employee_id)=(a.org_id,a.employee_id)
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND m.started_at IS NOT NULL
 AND(a.valid_to IS NULL OR GREATEST(a.valid_from,m.started_at)<a.valid_to)
 AND(m.ended_at IS NULL OR GREATEST(a.valid_from,m.started_at)<m.ended_at)
ORDER BY 1,2 NULLS LAST;
