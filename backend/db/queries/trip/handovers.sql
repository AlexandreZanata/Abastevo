-- EMP-TRIP-012 driver handovers (migration 000243, B-BR-EMP-394..398).

-- name: ListTripHandovers :many
SELECT h.revision,h.operation_id::text AS operation_id,h.request_hash,h.from_membership_id::text AS from_membership_id,h.from_employee_id::text AS from_employee_id,
 h.to_membership_id::text AS to_membership_id,h.to_employee_id::text AS to_employee_id,h.recorder_membership_id::text AS recorder_membership_id,
 h.recorder_employee_id::text AS recorder_employee_id,h.reason,h.handed_over_at
FROM org_trip_handovers h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY h.revision;

-- name: TripHandoverByOperation :one
SELECT h.trip_id::text AS trip_id,h.revision,h.request_hash,h.recorder_membership_id::text AS recorder_membership_id
FROM org_trip_handovers h WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND h.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: TripHandoverReceiver :one
-- The receiving membership's employee record and states; no row is an
-- unknown or another organization's membership (forced RLS).
SELECT m.employee_id::text AS employee_id,m.account_id::text AS account_id,m.state AS membership_state,e.state AS employee_state,(e.merged_into IS NULL)::boolean AS unmerged,
 COALESCE(org_sync_review_employee_canonical(m.org_id,m.employee_id)::text,'')::text AS canonical_employee_id
FROM org_memberships m JOIN org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: TripHandoverNow :one
SELECT clock_timestamp()::timestamptz AS now;

-- name: InsertTripHandover :exec
INSERT INTO org_trip_handovers(org_id,trip_id,revision,operation_id,request_hash,from_membership_id,from_employee_id,to_membership_id,to_employee_id,
 recorder_membership_id,recorder_employee_id,reason)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(revision)::bigint,CAST(sqlc.arg(operation_id)::text AS uuid),
 sqlc.arg(request_hash)::text,CAST(sqlc.arg(from_membership_id)::text AS uuid),CAST(sqlc.arg(from_employee_id)::text AS uuid),
 CAST(sqlc.arg(to_membership_id)::text AS uuid),CAST(sqlc.arg(to_employee_id)::text AS uuid),CAST(sqlc.arg(recorder_membership_id)::text AS uuid),
 CAST(sqlc.arg(recorder_employee_id)::text AS uuid),sqlc.arg(reason)::text);

-- name: OrganizationTripHandovers :many
-- The organization's handovers in recording order after one handover operation.
SELECT h.trip_id::text AS trip_id,t.asset_id::text AS asset_id,h.revision,h.operation_id::text AS operation_id,h.request_hash,
 h.from_membership_id::text AS from_membership_id,h.from_employee_id::text AS from_employee_id,h.to_membership_id::text AS to_membership_id,
 h.to_employee_id::text AS to_employee_id,h.recorder_membership_id::text AS recorder_membership_id,h.recorder_employee_id::text AS recorder_employee_id,
 h.reason,h.handed_over_at
FROM org_trip_handovers h JOIN org_trips t ON(t.org_id,t.id)=(h.org_id,h.trip_id)
WHERE h.org_id=CAST(sqlc.arg(org_id)::text AS uuid)
 AND(sqlc.arg(after_id)::text='' OR(h.handed_over_at,h.operation_id)>(SELECT a.handed_over_at,a.operation_id FROM org_trip_handovers a
  WHERE a.org_id=h.org_id AND a.operation_id=CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)))
ORDER BY h.handed_over_at,h.operation_id
LIMIT sqlc.arg(row_limit)::integer;

-- name: OwnOpenTrips :many
-- The open trips whose current responsible member is the membership, by trip identity.
SELECT t.id::text AS id,t.asset_id::text AS asset_id,t.state,t.occurred_at,t.occurred_submicro,t.reporting_day::text AS reporting_day,
 org_trip_revision(t.org_id,t.id)::bigint AS revision
FROM org_trips t
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.state IN('IN_PROGRESS','PAUSED')
 AND org_trip_responsible_member(t.org_id,t.id)=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND(sqlc.arg(after_id)::text='' OR t.id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY t.id
LIMIT sqlc.arg(row_limit)::integer;
