-- name: TripEventFacts :one
-- EMP-TRIP-003: the operations already holding an event identity, read after
-- the root and asset locks.
SELECT COALESCE((SELECT e.operation_id::text FROM org_trip_events e WHERE e.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND e.id=CAST(sqlc.arg(id)::text AS uuid)),'')::text AS existing_operation,
 COALESCE((SELECT h.operation_id::text FROM org_trip_held_events h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.id=CAST(sqlc.arg(id)::text AS uuid)),'')::text AS held_operation;

-- name: InsertTripEvent :exec
INSERT INTO org_trip_events(org_id,id,trip_id,operation_id,payload_digest,event_type,event_text,author_membership_id,author_employee_id,enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),
 sqlc.arg(payload_digest)::text,sqlc.arg(event_type)::text,sqlc.narg(event_text)::text,CAST(sqlc.arg(author_membership_id)::text AS uuid),
 CAST(sqlc.arg(author_employee_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),sqlc.arg(occurred_at)::timestamptz,
 sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint);

-- name: GetTripEvent :one
SELECT id::text AS id,trip_id::text AS trip_id,operation_id::text AS operation_id,payload_digest,event_type,event_text,
 author_membership_id::text AS author_membership_id,author_employee_id::text AS author_employee_id,enrollment_id::text AS enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at
FROM org_trip_events WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ListTripEvents :many
-- The accepted logbook facts of one trip (held events are not facts).
SELECT id::text AS id,trip_id::text AS trip_id,operation_id::text AS operation_id,payload_digest,event_type,event_text,
 author_membership_id::text AS author_membership_id,author_employee_id::text AS author_employee_id,enrollment_id::text AS enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at
FROM org_trip_events WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY occurred_at,occurred_submicro,id;

-- name: InsertTripHeldEvent :exec
INSERT INTO org_trip_held_events(org_id,review_item_id,operation_id,id,trip_id,event_type,event_text,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),
 CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(event_type)::text,sqlc.narg(event_text)::text,
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint);

-- name: ReadTripEventHeldProof :one
-- One held event of an operation with its review item and receipt.
SELECT i.id::text AS id,i.operation_type,i.operation_schema,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,i.proof_version,
 o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,o.entity_type,o.entity_id::text AS entity_id,
 h.id::text AS event_id,h.trip_id::text AS trip_id,h.event_type,h.event_text,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,
 COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,
 COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_sync_review_items i
JOIN org_trip_held_events h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);
