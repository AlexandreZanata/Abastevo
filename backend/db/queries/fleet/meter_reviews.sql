-- name: InsertMeterReviewItem :execrows
INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id)
SELECT o.org_id,CAST(sqlc.arg(id)::text AS uuid),o.operation_id,o.operation_type,o.operation_schema,o.payload_digest,sqlc.arg(code)::text,m.employee_id
FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND o.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertMeterHeldReading :exec
INSERT INTO org_meter_held_readings(org_id,review_item_id,operation_id,reading_id,asset_id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,origin,method,request_hash,unit_id)
SELECT a.org_id,CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(reading_id)::text AS uuid),a.id,CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(segment_id)::text AS uuid),sqlc.arg(value)::bigint,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,sqlc.arg(origin)::text,sqlc.arg(method)::text,sqlc.arg(request_hash)::text,a.unit_id
FROM org_assets a WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid);

-- name: ReadMeterHeldProof :one
SELECT i.id::text AS id,i.operation_id::text AS operation_id,i.operation_type,i.operation_schema,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,i.proof_version,
o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,o.entity_type,o.entity_id::text AS entity_id,
h.reading_id::text AS reading_id,h.asset_id::text AS asset_id,h.meter_id::text AS meter_id,h.segment_id::text AS segment_id,h.value,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,h.origin,h.method,h.request_hash,
COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_sync_review_items i JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: ReadMeterReview :one
-- review-expiry-v1 is elapsed 60x86400 seconds (EDR-119), never session-zone civil days.
SELECT i.id::text AS id,i.operation_id::text AS operation_id,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,
o.owner_account_id::text AS owner_account_id,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,
a.received_at,(a.received_at+make_interval(secs=>i.expiry_days::int*86400))::timestamptz AS expires_at,
h.asset_id::text AS asset_id,h.meter_id::text AS meter_id,h.segment_id::text AS segment_id,h.reading_id::text AS reading_id,h.value,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,h.origin,h.method,
COALESCE(z.canonical_resolution,''::bytea)::bytea AS canonical_resolution,COALESCE(z.outcome,'')::text AS outcome
FROM org_sync_review_items i JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
LEFT JOIN org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.id=CAST(sqlc.arg(id)::text AS uuid) AND i.operation_type='meter.reading.record' AND i.operation_schema=1;

-- name: ListMeterReviewIDs :many
-- Uses the same elapsed review-expiry-v1 deadline as ReadMeterReview.
SELECT i.id::text AS id FROM org_sync_review_items i
JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id)
LEFT JOIN org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_type='meter.reading.record' AND i.operation_schema=1
AND(sqlc.arg(after_id)::text='' OR i.id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
AND(sqlc.arg(state)::text='' OR CASE WHEN z.outcome='ACCEPTED' THEN 'RESOLVED_ACCEPTED' WHEN z.outcome='REFUSED' THEN 'RESOLVED_REFUSED' WHEN a.received_at+make_interval(secs=>i.expiry_days::int*86400)<=sqlc.arg(at)::timestamptz THEN 'EXPIRED_UNRESOLVED' ELSE 'OPEN' END=sqlc.arg(state)::text)
ORDER BY i.id LIMIT sqlc.arg(row_limit)::int;

-- name: LockMeterReview :one
SELECT id::text AS id FROM org_sync_review_items WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: MeterReviewDecisionReceipt :one
SELECT review_item_id::text AS review_item_id,reviewer_account_id::text AS reviewer_account_id,request_hash,canonical_resolution
FROM org_sync_review_resolutions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertMeterReviewResolution :exec
INSERT INTO org_sync_review_resolutions(org_id,id,review_item_id,expected_revision,outcome,reason,reviewer_account_id,reviewer_membership_id,reviewer_employee_id,reviewer_session_id,request_hash,resolved_at,entity_revision,reading_id,canonical_resolution)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),sqlc.arg(expected_revision)::bigint,sqlc.arg(outcome)::text,sqlc.arg(reason)::text,CAST(sqlc.arg(reviewer_account_id)::text AS uuid),CAST(sqlc.arg(reviewer_membership_id)::text AS uuid),CAST(sqlc.arg(reviewer_employee_id)::text AS uuid),CAST(sqlc.arg(reviewer_session_id)::text AS uuid),sqlc.arg(request_hash)::text,sqlc.arg(resolved_at)::timestamptz,sqlc.narg(entity_revision)::bigint,CAST(NULLIF(sqlc.arg(reading_id)::text,'') AS uuid),sqlc.arg(canonical_resolution)::bytea);

-- name: ReadMeterReviewResolution :one
SELECT z.review_item_id::text AS review_item_id,z.outcome,z.canonical_resolution,i.operation_id::text AS operation_id,h.asset_id::text AS asset_id,h.meter_id::text AS meter_id,h.segment_id::text AS segment_id,h.reading_id::text AS reading_id
FROM org_sync_review_resolutions z JOIN org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id) JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id)
WHERE z.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND z.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertReviewedMeterReading :exec
INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,enrollment_id,operation_id,request_hash,origin,method,state,review_resolution_id)
SELECT h.org_id,h.reading_id,h.meter_id,h.segment_id,h.value,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,o.membership_id,'ENROLLED_DEVICE',o.enrollment_id,o.operation_id,h.request_hash,h.origin,h.method,'VALID',z.id
FROM org_sync_review_resolutions z JOIN org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id) JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
WHERE z.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND z.id=CAST(sqlc.arg(id)::text AS uuid) AND z.outcome='ACCEPTED';

-- name: InsertMeterReviewEffect :exec
INSERT INTO org_sync_effects(org_id,operation_id,entity_revision)VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),1);

-- name: CanonicalReviewEmployee :one
SELECT COALESCE(org_sync_review_employee_canonical(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid))::text,'')::text AS id;

-- name: OwnMeterReviewResolution :one
SELECT COALESCE(z.canonical_resolution,''::bytea)::bytea AS canonical_resolution
FROM org_sync_review_items i LEFT JOIN org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.id=CAST(sqlc.arg(id)::text AS uuid);
