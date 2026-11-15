-- EMP-TRIP-014 supervisor closure (B-BR-EMP-399..403, EDR-258/259).

-- name: InsertTripClosure :one
-- The insert completes the trip; the database re-proves the trip in
-- progress, the closer and the separation, and publishes the closure only
-- with its audit event.
INSERT INTO org_trip_closures(org_id,trip_id,operation_id,request_hash,closer_membership_id,closer_employee_id,reason,missing_reading_reason)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text,
 CAST(sqlc.arg(closer_membership_id)::text AS uuid),CAST(sqlc.arg(closer_employee_id)::text AS uuid),sqlc.arg(reason)::text,sqlc.arg(missing_reading_reason)::text)
RETURNING closed_at;

-- name: GetTripClosure :one
SELECT trip_id::text AS trip_id,operation_id::text AS operation_id,request_hash,closer_membership_id::text AS closer_membership_id,
 closer_employee_id::text AS closer_employee_id,reason,missing_reading_reason,closed_at
FROM org_trip_closures WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND trip_id=CAST(sqlc.arg(trip_id)::text AS uuid);

-- name: TripClosureByOperation :one
SELECT trip_id::text AS trip_id,operation_id::text AS operation_id,request_hash,closer_membership_id::text AS closer_membership_id,
 closer_employee_id::text AS closer_employee_id,reason,missing_reading_reason,closed_at
FROM org_trip_closures WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: OpenTrips :many
-- Trips in progress, oldest start first, after one trip (keyset).
SELECT t.id::text AS id,t.asset_id::text AS asset_id,t.driver_employee_id::text AS driver_employee_id,t.author_membership_id::text AS author_membership_id,
 t.occurred_at,t.occurred_submicro,t.reporting_day::text AS reporting_day
FROM org_trips t
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.state='IN_PROGRESS'
 AND(sqlc.arg(after_id)::text='' OR(t.occurred_at,t.occurred_submicro,t.id)>(SELECT a.occurred_at,a.occurred_submicro,a.id FROM org_trips a
  WHERE a.org_id=t.org_id AND a.id=CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)))
ORDER BY t.occurred_at,t.occurred_submicro,t.id
LIMIT sqlc.arg(row_limit)::integer;
