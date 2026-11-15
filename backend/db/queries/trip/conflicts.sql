-- EMP-TRIP-007 conflict queue and resolution (B-BR-EMP-407..411, EDR-262..264).

-- name: ListTripConflictIDs :many
-- Held trip items in identifier order; the state uses the elapsed
-- review-expiry-v1 deadline (EDR-119) like the meter queue.
SELECT i.id::text AS id FROM org_sync_review_items i
JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
LEFT JOIN org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_schema=1 AND i.operation_type IN('trip.start','trip.finish','trip.event.record','trip.submit','trip.pause','trip.resume','trip.cancel')
AND(sqlc.arg(after_id)::text='' OR i.id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
AND(sqlc.arg(state)::text='' OR CASE WHEN z.outcome='ACCEPTED' THEN 'RESOLVED_ACCEPTED' WHEN z.outcome='REFUSED' THEN 'RESOLVED_REFUSED' WHEN a.received_at+make_interval(secs=>i.expiry_days::int*86400)<=sqlc.arg(at)::timestamptz THEN 'EXPIRED_UNRESOLVED' ELSE 'OPEN' END=sqlc.arg(state)::text)
ORDER BY i.id LIMIT sqlc.arg(row_limit)::int;

-- name: ReadTripConflictItem :one
SELECT i.id::text AS id,i.operation_id::text AS operation_id,i.operation_type,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,
 o.owner_account_id::text AS owner_account_id,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,
 a.received_at,(a.received_at+make_interval(secs=>i.expiry_days::int*86400))::timestamptz AS expires_at,
 COALESCE(z.canonical_resolution,''::bytea)::bytea AS canonical_resolution,COALESCE(z.outcome,'')::text AS outcome
FROM org_sync_review_items i JOIN org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
LEFT JOIN org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.id=CAST(sqlc.arg(id)::text AS uuid) AND i.operation_schema=1
AND i.operation_type IN('trip.start','trip.finish','trip.event.record','trip.submit','trip.pause','trip.resume','trip.cancel');

-- name: ReadTripConflictStart :one
SELECT h.trip_id::text AS trip_id,h.asset_id::text AS asset_id,h.driver_employee_id::text AS driver_employee_id,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,
 h.boot_id,h.elapsed_millis,COALESCE(h.meter_id::text,'')::text AS meter_id,COALESCE(h.segment_id::text,'')::text AS segment_id,COALESCE(h.reading_id::text,'')::text AS reading_id,
 h.reading_value,h.missing_reading_reason,COALESCE(m.kind,'')::text AS reading_kind
FROM org_trip_held_starts h LEFT JOIN org_meters m ON(m.org_id,m.id)=(h.org_id,h.meter_id)
WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.review_item_id=CAST(sqlc.arg(review_item_id)::text AS uuid);

-- name: ReadTripConflictFinish :one
SELECT h.trip_id::text AS trip_id,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,h.boot_id,h.elapsed_millis,
 COALESCE(h.meter_id::text,'')::text AS meter_id,COALESCE(h.segment_id::text,'')::text AS segment_id,COALESCE(h.reading_id::text,'')::text AS reading_id,
 h.reading_value,h.missing_reading_reason,COALESCE(m.kind,'')::text AS reading_kind
FROM org_trip_held_finishes h LEFT JOIN org_meters m ON(m.org_id,m.id)=(h.org_id,h.meter_id)
WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.review_item_id=CAST(sqlc.arg(review_item_id)::text AS uuid);

-- name: ReadTripConflictEvent :one
SELECT h.trip_id::text AS trip_id,h.id::text AS event_id,h.event_type,h.event_text,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds
FROM org_trip_held_events h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.review_item_id=CAST(sqlc.arg(review_item_id)::text AS uuid);

-- name: ReadTripConflictSubmission :one
SELECT h.trip_id::text AS trip_id,h.revision,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds
FROM org_trip_held_submissions h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.review_item_id=CAST(sqlc.arg(review_item_id)::text AS uuid);

-- name: TripConflictTripsOfAsset :many
-- B-BR-EMP-409: the other trips of the asset in progress or paused, or
-- whose end (its finish, or its cancellation under EMP-TRIP-011) is after
-- the held start's declared instant, in start order.
SELECT t.id::text AS id,t.state,t.occurred_at,t.occurred_submicro,COALESCE(f.occurred_at,c.occurred_at)::timestamptz AS finished_at,COALESCE(f.occurred_submicro,c.occurred_submicro,0)::smallint AS finished_submicro
FROM org_trips t LEFT JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id)
LEFT JOIN org_trip_transitions c ON(c.org_id,c.trip_id,c.command)=(t.org_id,t.id,'CANCEL')
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND t.id<>CAST(sqlc.arg(trip_id)::text AS uuid)
AND(t.state NOT IN('COMPLETED','CANCELLED') OR(COALESCE(f.occurred_at,c.occurred_at),COALESCE(f.occurred_submicro,c.occurred_submicro))>(sqlc.arg(at)::timestamptz,sqlc.arg(at_submicro)::smallint))
ORDER BY t.occurred_at,t.occurred_submicro,t.id;

-- name: TripConflictTrip :one
-- One trip with its end: its finish, or its cancellation (EMP-TRIP-011).
SELECT t.id::text AS id,t.state,t.occurred_at,t.occurred_submicro,COALESCE(f.occurred_at,c.occurred_at)::timestamptz AS finished_at,COALESCE(f.occurred_submicro,c.occurred_submicro,0)::smallint AS finished_submicro
FROM org_trips t LEFT JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id)
LEFT JOIN org_trip_transitions c ON(c.org_id,c.trip_id,c.command)=(t.org_id,t.id,'CANCEL')
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ReadTripConflictTransition :one
-- A held EMP-TRIP-011 lifecycle command.
SELECT h.trip_id::text AS trip_id,h.command,h.expected_revision,h.reason,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds
FROM org_trip_held_transitions h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.review_item_id=CAST(sqlc.arg(review_item_id)::text AS uuid);

-- name: LockTripConflict :one
SELECT operation_type FROM org_sync_review_items WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: TripConflictDecisionReceipt :one
SELECT review_item_id::text AS review_item_id,reviewer_account_id::text AS reviewer_account_id,request_hash,canonical_resolution
FROM org_sync_review_resolutions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: TripConflictFacts :one
-- The held start's asset under its row lock, its author's current membership
-- and enrolled device and its driver (B-BR-EMP-409).
SELECT a.state='ACTIVE' AS asset_active,a.disposed_at,a.disposed_submicro,
 EXISTS(SELECT 1 FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=a.org_id AND o.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid) AND m.state='ACTIVE' AND e.state='ENROLLED' AND m.account_id=o.owner_account_id
  AND m.employee_id=CAST(sqlc.arg(author_employee_id)::text AS uuid))::boolean AS author_current,
 EXISTS(SELECT 1 FROM org_employee_identities d WHERE d.org_id=a.org_id AND d.id=CAST(sqlc.arg(driver_id)::text AS uuid) AND d.state='ACTIVE' AND d.merged_into IS NULL)::boolean AS driver_active,
 (SELECT g.time_zone FROM organizations g WHERE g.org_id=a.org_id)::text AS organization_zone
FROM org_assets a WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid) FOR UPDATE OF a;

-- name: InsertTripConflictResolution :exec
INSERT INTO org_sync_review_resolutions(org_id,id,review_item_id,expected_revision,outcome,reason,reviewer_account_id,reviewer_membership_id,reviewer_employee_id,reviewer_session_id,request_hash,resolved_at,entity_revision,reading_id,canonical_resolution)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),sqlc.arg(expected_revision)::bigint,sqlc.arg(outcome)::text,sqlc.arg(reason)::text,
 CAST(sqlc.arg(reviewer_account_id)::text AS uuid),CAST(sqlc.arg(reviewer_membership_id)::text AS uuid),CAST(sqlc.arg(reviewer_employee_id)::text AS uuid),CAST(sqlc.arg(reviewer_session_id)::text AS uuid),
 sqlc.arg(request_hash)::text,sqlc.arg(resolved_at)::timestamptz,sqlc.narg(entity_revision)::bigint,CAST(NULLIF(sqlc.arg(reading_id)::text,'') AS uuid),sqlc.arg(canonical_resolution)::bytea);

-- name: InsertTripConflictEffect :exec
INSERT INTO org_sync_effects(org_id,operation_id,entity_revision)VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),1);

-- name: TripConflictNow :one
SELECT clock_timestamp()::timestamptz AS now;
