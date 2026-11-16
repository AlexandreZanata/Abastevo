-- EMP-TRIP-007 (EDR-263/264): owner-neutral OFFL-008 review support.

-- name: ReadOwnReviewProof :one
-- The author's own result read proves the review item, its operation and its
-- acknowledgement for every owner; the held row of the owner was proven at
-- the item's commit and is immutable.
SELECT i.id::text AS id,i.code,i.proof_version,i.payload_digest,
o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,
COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at
FROM org_sync_review_items i JOIN org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertReviewedEmbeddedReading :exec
-- The embedded reading of an accepted held field fact of another owner; the
-- observation guard re-proves the resolution and stamps the original receipt.
INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,enrollment_id,operation_id,request_hash,origin,method,state,review_resolution_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(segment_id)::text AS uuid),sqlc.arg(value)::bigint,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,CAST(sqlc.arg(author_membership_id)::text AS uuid),'ENROLLED_DEVICE',CAST(sqlc.arg(enrollment_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text,sqlc.arg(origin)::text,sqlc.arg(method)::text,sqlc.arg(state)::text,CAST(sqlc.arg(review_resolution_id)::text AS uuid));
