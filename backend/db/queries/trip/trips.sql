-- name: TripAuthorEmployee :one
-- EMP-TRIP-002: the author's employee record, read in the intake transaction.
SELECT employee_id::text AS employee_id FROM org_memberships
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: TripStartFacts :one
-- Admission facts of one start, taken after the root and asset locks.
SELECT a.state AS asset_state,a.kind AS asset_kind,a.disposed_at,a.disposed_submicro,
 EXISTS(SELECT 1 FROM org_employee_identities e WHERE e.org_id=a.org_id AND e.id=CAST(sqlc.arg(driver_id)::text AS uuid) AND e.state='ACTIVE' AND e.merged_into IS NULL)::boolean AS driver_active,
 COALESCE((SELECT t.operation_id::text FROM org_trips t WHERE t.org_id=a.org_id AND t.id=CAST(sqlc.arg(trip_id)::text AS uuid)),'')::text AS existing_operation,
 COALESCE((SELECT h.operation_id::text FROM org_trip_held_starts h WHERE h.org_id=a.org_id AND h.trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)),'')::text AS held_operation,
 COALESCE((SELECT t.id::text FROM org_trips t WHERE t.org_id=a.org_id AND t.asset_id=a.id AND t.state='IN_PROGRESS'),'')::text AS active_trip,
 COALESCE(o.time_zone,'')::text AS organization_zone
FROM org_assets a JOIN organizations o ON o.org_id=a.org_id
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid);

-- name: TripUnitZone :one
SELECT time_zone FROM org_units WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(unit_id)::text AS uuid);

-- name: InsertTrip :exec
INSERT INTO org_trips(org_id,id,operation_id,payload_digest,asset_id,driver_employee_id,author_membership_id,author_employee_id,enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds,boot_id,elapsed_millis,meter_id,segment_id,reading_id,missing_reading_reason,
 unit_id,cost_centre_id,reporting_day,reporting_zone,reporting_source,state)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(driver_employee_id)::text AS uuid),CAST(sqlc.arg(author_membership_id)::text AS uuid),
 CAST(sqlc.arg(author_employee_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,
 sqlc.narg(boot_id)::text,sqlc.narg(elapsed_millis)::bigint,CAST(sqlc.narg(meter_id)::text AS uuid),CAST(sqlc.narg(segment_id)::text AS uuid),
 CAST(sqlc.narg(reading_id)::text AS uuid),sqlc.narg(missing_reading_reason)::text,CAST(sqlc.narg(unit_id)::text AS uuid),CAST(sqlc.narg(cost_centre_id)::text AS uuid),
 sqlc.arg(reporting_day)::date,sqlc.arg(reporting_zone)::text,sqlc.arg(reporting_source)::text,'IN_PROGRESS');

-- name: InsertTripFinish :exec
INSERT INTO org_trip_finishes(org_id,trip_id,operation_id,payload_digest,author_membership_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,
 device_offset_seconds,boot_id,elapsed_millis,meter_id,segment_id,reading_id,missing_reading_reason,end_day)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(author_membership_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,
 sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,sqlc.narg(boot_id)::text,sqlc.narg(elapsed_millis)::bigint,
 CAST(sqlc.narg(meter_id)::text AS uuid),CAST(sqlc.narg(segment_id)::text AS uuid),CAST(sqlc.narg(reading_id)::text AS uuid),
 sqlc.narg(missing_reading_reason)::text,sqlc.arg(end_day)::date);

-- name: GetTrip :one
SELECT t.id::text AS id,t.operation_id::text AS operation_id,t.payload_digest,t.asset_id::text AS asset_id,t.driver_employee_id::text AS driver_employee_id,
 t.author_membership_id::text AS author_membership_id,t.author_employee_id::text AS author_employee_id,t.enrollment_id::text AS enrollment_id,
 t.occurred_at,t.occurred_submicro,t.entry_zone,t.device_offset_seconds,t.boot_id,t.elapsed_millis,t.received_at,
 COALESCE(t.meter_id::text,'')::text AS meter_id,COALESCE(t.segment_id::text,'')::text AS segment_id,COALESCE(t.reading_id::text,'')::text AS reading_id,
 COALESCE(m.kind,'')::text AS reading_kind,r.value AS reading_value,t.missing_reading_reason,
 COALESCE(t.unit_id::text,'')::text AS unit_id,COALESCE(t.cost_centre_id::text,'')::text AS cost_centre_id,
 t.reporting_day::text AS reporting_day,t.reporting_zone,t.reporting_source,t.state,
 COALESCE(f.operation_id::text,'')::text AS finish_operation_id,COALESCE(f.payload_digest,'')::text AS finish_payload_digest,
 COALESCE(f.author_membership_id::text,'')::text AS finish_author_membership_id,COALESCE(f.enrollment_id::text,'')::text AS finish_enrollment_id,
 f.occurred_at AS finish_occurred_at,f.occurred_submicro AS finish_occurred_submicro,f.entry_zone AS finish_entry_zone,f.device_offset_seconds AS finish_device_offset_seconds,
 f.boot_id AS finish_boot_id,f.elapsed_millis AS finish_elapsed_millis,f.received_at AS finish_received_at,
 COALESCE(f.meter_id::text,'')::text AS finish_meter_id,COALESCE(f.segment_id::text,'')::text AS finish_segment_id,COALESCE(f.reading_id::text,'')::text AS finish_reading_id,
 COALESCE(fm.kind,'')::text AS finish_reading_kind,fr.value AS finish_reading_value,
 f.missing_reading_reason AS finish_missing_reading_reason,COALESCE(f.end_day::text,'')::text AS finish_end_day
FROM org_trips t
LEFT JOIN org_meters m ON(m.org_id,m.id)=(t.org_id,t.meter_id)
LEFT JOIN org_meter_readings r ON(r.org_id,r.meter_id,r.id)=(t.org_id,t.meter_id,t.reading_id)
LEFT JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id)
LEFT JOIN org_meters fm ON(fm.org_id,fm.id)=(f.org_id,f.meter_id)
LEFT JOIN org_meter_readings fr ON(fr.org_id,fr.meter_id,fr.id)=(f.org_id,f.meter_id,f.reading_id)
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertTripReviewItem :execrows
INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id)
SELECT o.org_id,CAST(sqlc.arg(id)::text AS uuid),o.operation_id,o.operation_type,o.operation_schema,o.payload_digest,sqlc.arg(code)::text,m.employee_id
FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND o.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertTripHeldStart :exec
INSERT INTO org_trip_held_starts(org_id,review_item_id,operation_id,trip_id,asset_id,driver_employee_id,occurred_at,occurred_submicro,entry_zone,
 device_offset_seconds,boot_id,elapsed_millis,meter_id,segment_id,reading_id,reading_value,missing_reading_reason)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),
 CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(driver_employee_id)::text AS uuid),sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,
 sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,sqlc.narg(boot_id)::text,sqlc.narg(elapsed_millis)::bigint,
 CAST(sqlc.narg(meter_id)::text AS uuid),CAST(sqlc.narg(segment_id)::text AS uuid),CAST(sqlc.narg(reading_id)::text AS uuid),sqlc.narg(reading_value)::bigint,
 sqlc.narg(missing_reading_reason)::text);

-- name: InsertTripHeldFinish :exec
INSERT INTO org_trip_held_finishes(org_id,review_item_id,operation_id,trip_id,occurred_at,occurred_submicro,entry_zone,
 device_offset_seconds,boot_id,elapsed_millis,meter_id,segment_id,reading_id,reading_value,missing_reading_reason)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(review_item_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,
 sqlc.narg(boot_id)::text,sqlc.narg(elapsed_millis)::bigint,CAST(sqlc.narg(meter_id)::text AS uuid),CAST(sqlc.narg(segment_id)::text AS uuid),
 CAST(sqlc.narg(reading_id)::text AS uuid),sqlc.narg(reading_value)::bigint,sqlc.narg(missing_reading_reason)::text);

-- name: ReadTripHeldProof :one
-- One held start or finish of an operation with its review item and receipt.
SELECT i.id::text AS id,i.operation_type,i.operation_schema,i.payload_digest,i.code,i.author_employee_id::text AS author_employee_id,i.proof_version,
 o.owner_account_id::text AS owner_account_id,o.api_origin,o.membership_id::text AS membership_id,o.enrollment_id::text AS enrollment_id,o.entity_type,o.entity_id::text AS entity_id,
 h.trip_id::text AS trip_id,h.asset_id::text AS asset_id,h.driver_employee_id::text AS driver_employee_id,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,
 h.boot_id,h.elapsed_millis,COALESCE(h.meter_id::text,'')::text AS meter_id,COALESCE(h.segment_id::text,'')::text AS segment_id,COALESCE(h.reading_id::text,'')::text AS reading_id,
 h.reading_value,h.missing_reading_reason,
 COALESCE(a.result,'')::text AS acknowledgement_result,COALESCE(a.code,'')::text AS acknowledgement_code,COALESCE(a.review_item_id::text,'')::text AS acknowledgement_review_item_id,
 COALESCE(a.review_proof_version,0)::smallint AS acknowledgement_proof_version,a.received_at,COALESCE(a.canonical_ack,''::bytea)::bytea AS canonical_ack
FROM org_sync_review_items i
JOIN (SELECT s.org_id,s.review_item_id,s.operation_id,s.trip_id,s.asset_id::text AS asset_id,s.driver_employee_id::text AS driver_employee_id,s.occurred_at,s.occurred_submicro,s.entry_zone,
  s.device_offset_seconds,s.boot_id,s.elapsed_millis,s.meter_id,s.segment_id,s.reading_id,s.reading_value,s.missing_reading_reason FROM org_trip_held_starts s
 UNION ALL SELECT f.org_id,f.review_item_id,f.operation_id,f.trip_id,'','',f.occurred_at,f.occurred_submicro,f.entry_zone,
  f.device_offset_seconds,f.boot_id,f.elapsed_millis,f.meter_id,f.segment_id,f.reading_id,f.reading_value,f.missing_reading_reason FROM org_trip_held_finishes f) h
 ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
LEFT JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
WHERE i.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND i.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: TripInProgressOnAsset :one
-- FLEET-003 open work: an IN_PROGRESS trip of the asset (B-BR-EMP-318).
SELECT EXISTS(SELECT 1 FROM org_trips WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND state='IN_PROGRESS')::boolean AS open;

-- name: TripUsesOfAssetAt :many
-- FLEET-007 active use (EDR-206): trips holding the asset at the instant.
-- A trip without a finish holds it only while IN_PROGRESS: a trip closed by
-- a supervisor gets no invented end instant (EMP-TRIP-014, EDR-258).
SELECT t.id::text AS id,t.occurred_at,t.occurred_submicro FROM org_trips t LEFT JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id)
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid)
AND(t.occurred_at,t.occurred_submicro)<=(sqlc.arg(at)::timestamptz,sqlc.arg(at_submicro)::smallint)
AND(f.trip_id IS NULL AND t.state='IN_PROGRESS' OR(f.occurred_at,f.occurred_submicro)>(sqlc.arg(at)::timestamptz,sqlc.arg(at_submicro)::smallint))
ORDER BY t.occurred_at,t.occurred_submicro,t.id;
