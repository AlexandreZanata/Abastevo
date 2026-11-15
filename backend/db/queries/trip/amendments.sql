-- name: TripAmendments :many
-- EMP-TRIP-015: the amendment revisions of one trip in revision order.
SELECT revision,base_revision,intent,COALESCE(start_meter_id::text,'')::text AS start_meter_id,COALESCE(start_reading_id::text,'')::text AS start_reading_id,
 start_from,start_to,COALESCE(start_new_reading_id::text,'')::text AS start_new_reading_id,
 COALESCE(end_meter_id::text,'')::text AS end_meter_id,COALESCE(end_reading_id::text,'')::text AS end_reading_id,
 end_from,end_to,COALESCE(end_new_reading_id::text,'')::text AS end_new_reading_id,
 actor_membership_id::text AS actor_membership_id,actor_employee_id::text AS actor_employee_id,operation_id::text AS operation_id,request_hash,recorded_at
FROM org_trip_amendments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY revision;

-- name: TripAmendmentByOperation :one
SELECT trip_id::text AS trip_id,revision,request_hash,actor_membership_id::text AS actor_membership_id
FROM org_trip_amendments
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertTripAmendment :exec
INSERT INTO org_trip_amendments(org_id,trip_id,revision,base_revision,intent,start_meter_id,start_reading_id,start_from,start_to,start_new_reading_id,
 end_meter_id,end_reading_id,end_from,end_to,end_new_reading_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(base_revision)::bigint,sqlc.arg(intent)::text,
 CAST(sqlc.narg(start_meter_id)::text AS uuid),CAST(sqlc.narg(start_reading_id)::text AS uuid),sqlc.narg(start_from)::bigint,sqlc.narg(start_to)::bigint,CAST(sqlc.narg(start_new_reading_id)::text AS uuid),
 CAST(sqlc.narg(end_meter_id)::text AS uuid),CAST(sqlc.narg(end_reading_id)::text AS uuid),sqlc.narg(end_from)::bigint,sqlc.narg(end_to)::bigint,CAST(sqlc.narg(end_new_reading_id)::text AS uuid),
 CAST(sqlc.arg(actor_membership_id)::text AS uuid),CAST(sqlc.arg(actor_employee_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text);

-- name: TripPointReadings :one
-- The current reading of each trip point in the effective revision, its
-- value and its meter's kind (000229 org_trip_point_reading).
SELECT COALESCE(s.meter_id::text,'')::text AS start_meter_id,COALESCE(s.id::text,'')::text AS start_reading_id,COALESCE(s.value,0)::bigint AS start_value,COALESCE(sm.kind,'')::text AS start_kind,
 COALESCE(e.meter_id::text,'')::text AS end_meter_id,COALESCE(e.id::text,'')::text AS end_reading_id,COALESCE(e.value,0)::bigint AS end_value,COALESCE(em.kind,'')::text AS end_kind
FROM(SELECT CAST(org_trip_point_reading(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),'START')::text AS uuid) AS sid,
 CAST(org_trip_point_reading(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),'END')::text AS uuid) AS eid) p
LEFT JOIN org_meter_readings s ON s.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND s.id=p.sid
LEFT JOIN org_meters sm ON(sm.org_id,sm.id)=(s.org_id,s.meter_id)
LEFT JOIN org_meter_readings e ON e.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND e.id=p.eid
LEFT JOIN org_meters em ON(em.org_id,em.id)=(e.org_id,e.meter_id);

-- name: TripReadingApprovedParent :one
-- METER-004 parent port (B-BR-EMP-368): the reading is a start, finish or
-- superseding amendment reading of a trip with an approved revision.
SELECT EXISTS(SELECT 1 FROM(SELECT t.id AS trip_id FROM org_trips t WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.reading_id=CAST(sqlc.arg(reading_id)::text AS uuid)
 UNION ALL SELECT f.trip_id FROM org_trip_finishes f WHERE f.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND f.reading_id=CAST(sqlc.arg(reading_id)::text AS uuid)
 UNION ALL SELECT a.trip_id FROM org_trip_amendments a WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid)
  AND CAST(sqlc.arg(reading_id)::text AS uuid) IN(a.start_new_reading_id,a.end_new_reading_id)) r
 WHERE EXISTS(SELECT 1 FROM org_trip_review_entries e WHERE e.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND e.trip_id=r.trip_id AND e.kind='APPROVE'))::boolean AS approved;

-- name: InsertTripHeldSubmission :exec
INSERT INTO org_trip_held_submissions(org_id,review_item_id,operation_id,trip_id,revision,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),
 CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,
 sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint);

-- name: ReadTripSubmitHeldProof :one
-- One held submission of an operation with its review item and receipt.
SELECT i.id::text AS id,i.operation_type,i.operation_schema,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,i.proof_version,
 o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,o.entity_type,o.entity_id::text AS entity_id,
 h.trip_id::text AS trip_id,h.revision,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,
 COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,
 COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_sync_review_items i
JOIN org_trip_held_submissions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);
