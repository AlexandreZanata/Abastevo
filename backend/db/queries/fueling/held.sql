-- name: CreateFuelingPlanReviewItem :execrows
INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id)
SELECT o.org_id,sqlc.arg('review_id')::text::uuid,o.operation_id,o.operation_type,o.operation_schema,
       o.payload_digest,'PLAN_CHANGED',m.employee_id
FROM org_sync_operations o JOIN org_memberships m ON (m.org_id,m.id,m.account_id)=(o.org_id,o.membership_id,o.owner_account_id)
WHERE (o.org_id,o.operation_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid)
  AND o.operation_type='fueling.submit' AND o.operation_schema=1 AND m.state='ACTIVE';

-- name: InsertHeldFueling :exec
INSERT INTO org_held_fuelings(org_id,review_item_id,operation_id,fueling_id,asset_id,responsible_employee_id,canonical_operation)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('review_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,
       sqlc.arg('fueling_id')::text::uuid,sqlc.arg('asset_id')::text::uuid,sqlc.arg('responsible_id')::text::uuid,
       sqlc.arg('canonical_operation')::bytea);

-- name: HeldFuelingExists :one
SELECT EXISTS (SELECT 1 FROM org_held_fuelings
WHERE (org_id,fueling_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid));

-- name: GetHeldFueling :one
SELECT i.id::text AS review_id,h.canonical_operation,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_held_fuelings h JOIN org_sync_review_items i ON (i.org_id,i.id,i.operation_id)=(h.org_id,h.review_item_id,h.operation_id)
JOIN org_sync_operations o ON (o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=
    (i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
LEFT JOIN org_sync_acknowledgements a ON (a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=
    (i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
WHERE (o.org_id,o.operation_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid)
  AND (o.owner_account_id,o.api_origin,o.membership_id,o.enrollment_id)=(
      sqlc.arg('account_id')::text::uuid,sqlc.arg('origin')::text,sqlc.arg('membership_id')::text::uuid,sqlc.arg('enrollment_id')::text::uuid)
  AND o.operation_type='fueling.submit' AND o.operation_schema=1 AND i.code='PLAN_CHANGED';
