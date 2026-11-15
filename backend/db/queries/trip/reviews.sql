-- name: TripReviewFacts :one
-- EMP-TRIP-004: the decision facts of one trip and its author's account.
SELECT t.id::text AS id,t.asset_id::text AS asset_id,t.driver_employee_id::text AS driver_employee_id,
 t.author_membership_id::text AS author_membership_id,t.author_employee_id::text AS author_employee_id,m.account_id::text AS author_account_id,
 t.state,t.occurred_at,t.occurred_submicro,t.reporting_day::text AS reporting_day,
 f.occurred_at AS finished_at,f.occurred_submicro AS finished_submicro
FROM org_trips t
JOIN org_memberships m ON(m.org_id,m.id)=(t.org_id,t.author_membership_id)
LEFT JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id)
WHERE t.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND t.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: LockTripForReview :one
-- The trip row lock serializes every review entry of one trip (EDR-221).
SELECT state FROM org_trips WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: TripReviewEntries :many
SELECT sequence,kind,revision,state,reason,actor_membership_id::text AS actor_membership_id,actor_employee_id::text AS actor_employee_id,
 operation_id::text AS operation_id,recorded_at
FROM org_trip_review_entries
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY sequence;

-- name: TripReviewOperationEntries :many
SELECT trip_id::text AS trip_id,sequence,kind,revision,state,reason,actor_membership_id::text AS actor_membership_id,
 actor_employee_id::text AS actor_employee_id,recorded_at
FROM org_trip_review_entries
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)
ORDER BY trip_id,sequence;

-- name: InsertTripReviewEntry :exec
INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(trip_id)::text AS uuid),sqlc.arg(sequence)::bigint,sqlc.arg(kind)::text,
 sqlc.arg(revision)::bigint,sqlc.arg(state)::text,sqlc.arg(reason)::text,CAST(sqlc.arg(actor_membership_id)::text AS uuid),
 CAST(sqlc.arg(actor_employee_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid));

-- name: TripReviewPerson :one
-- The canonical employee record (merge chain) and the account of a membership.
SELECT COALESCE(org_sync_review_employee_canonical(m.org_id,m.employee_id)::text,'')::text AS canonical_employee_id,
 m.employee_id::text AS employee_id,m.account_id::text AS account_id
FROM org_memberships m
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: TripReviewEmployee :one
SELECT COALESCE(org_sync_review_employee_canonical(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid))::text,'')::text AS canonical_employee_id;

-- name: TripReviewQueue :many
-- Trips whose latest entry is SUBMITTED or IN_REVIEW, in submission order.
SELECT t.id::text AS id,t.asset_id::text AS asset_id,t.driver_employee_id::text AS driver_employee_id,t.reporting_day::text AS reporting_day,
 h.state,h.revision,s.recorded_at AS submitted_at
FROM org_trip_review_entries s
JOIN org_trips t ON(t.org_id,t.id)=(s.org_id,s.trip_id)
JOIN LATERAL(SELECT e.state,e.revision FROM org_trip_review_entries e WHERE e.org_id=s.org_id AND e.trip_id=s.trip_id ORDER BY e.sequence DESC LIMIT 1) h ON true
WHERE s.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND s.kind='SUBMIT' AND h.state IN('SUBMITTED','IN_REVIEW')
 AND(sqlc.arg(after_id)::text='' OR(s.recorded_at,s.trip_id)>(SELECT a.recorded_at,a.trip_id FROM org_trip_review_entries a
  WHERE a.org_id=s.org_id AND a.kind='SUBMIT' AND a.trip_id=CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)))
ORDER BY s.recorded_at,s.trip_id
LIMIT sqlc.arg(row_limit)::integer;
