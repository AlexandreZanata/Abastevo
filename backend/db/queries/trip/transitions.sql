-- EMP-TRIP-011 lifecycle commands (migration 000233, B-BR-EMP-391..393).

-- name: TripTransitionFacts :one
-- The operations already holding this operation identity for the trip.
SELECT COALESCE((SELECT x.operation_id::text FROM org_trip_transitions x WHERE x.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND x.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)),'')::text AS existing_operation,
 COALESCE((SELECT h.operation_id::text FROM org_trip_held_transitions h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)),'')::text AS held_operation;

-- name: ListTripTransitions :many
SELECT x.trip_id::text AS trip_id,x.revision,x.command,x.from_state,x.to_state,x.reason,x.operation_id::text AS operation_id,x.payload_digest,
 x.author_membership_id::text AS author_membership_id,x.author_employee_id::text AS author_employee_id,x.enrollment_id::text AS enrollment_id,
 x.occurred_at,x.occurred_submicro,x.entry_zone,x.device_offset_seconds,x.received_at
FROM org_trip_transitions x WHERE x.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND x.trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY x.revision;

-- name: InsertTripTransition :exec
INSERT INTO org_trip_transitions(org_id,trip_id,revision,command,from_state,to_state,reason,operation_id,payload_digest,author_membership_id,author_employee_id,
 enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(command)::text,sqlc.arg(from_state)::text,
 sqlc.arg(to_state)::text,sqlc.narg(reason)::text,CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(author_membership_id)::text AS uuid),CAST(sqlc.arg(author_employee_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint);

-- name: InsertTripHeldTransition :exec
INSERT INTO org_trip_held_transitions(org_id,review_item_id,operation_id,trip_id,command,expected_revision,reason,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),
 sqlc.arg(command)::text,sqlc.arg(expected_revision)::bigint,sqlc.narg(reason)::text,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,
 sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint);

-- name: ReadTripTransitionHeldProof :one
-- One held lifecycle command of an operation with its review item and receipt.
SELECT i.id::text AS id,i.operation_type,i.operation_schema,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,i.proof_version,
 o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,o.entity_type,o.entity_id::text AS entity_id,
 h.trip_id::text AS trip_id,h.command,h.expected_revision,h.reason,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,
 COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,
 COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_sync_review_items i
JOIN org_trip_held_transitions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);
