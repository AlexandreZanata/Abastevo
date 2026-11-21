-- name: ReadStationPlanReview :one
SELECT station_id::text AS station_id,revision
FROM org_billing_station_plan_reviews
WHERE (org_id,approval_id,approved_operator)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('approval_id')::text::uuid,sqlc.arg('operator_id')::text::uuid)
  AND decision='APPROVED';

-- name: CheckStationPlanReview :one
SELECT EXISTS (
  SELECT 1 FROM org_billing_station_plan_reviews_at(sqlc.arg('org_id')::text::uuid,sqlc.arg('observed_at')::timestamptz) review
  WHERE (review.station_id,review.revision)=(sqlc.arg('station_id')::text::uuid,sqlc.arg('review_revision')::bigint)
)::boolean AS approved;

-- name: CountAdmittedStations :one
SELECT count(*)::bigint
FROM org_billing_station_plan_reviews_at(sqlc.arg('org_id')::text::uuid,sqlc.arg('observed_at')::timestamptz) review
JOIN org_billing_station_plan_admissions a ON (a.org_id,a.station_id,a.review_revision)=
  (sqlc.arg('org_id')::text::uuid,review.station_id,review.revision)
WHERE a.admitted_at<=sqlc.arg('observed_at')::timestamptz;

-- name: RecordStationPlanAdmission :execrows
INSERT INTO org_billing_station_plan_admissions(org_id,operation_id,station_id,review_revision)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('station_id')::text::uuid,sqlc.arg('review_revision')::bigint)
ON CONFLICT DO NOTHING;

-- name: ReadStationPlanAdmission :one
SELECT station_id::text AS station_id,review_revision,admitted_at FROM org_billing_station_plan_admissions
WHERE (org_id,operation_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid);

-- name: LockStationPlanOrganization :one
SELECT state::text FROM organizations WHERE org_id=sqlc.arg('org_id')::text::uuid AND has_snapshot FOR UPDATE;

-- name: StationAlreadyAdmitted :one
SELECT EXISTS (SELECT 1 FROM org_billing_station_plan_admissions
WHERE (org_id,station_id,review_revision)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('station_id')::text::uuid,sqlc.arg('review_revision')::bigint)
)::boolean AS admitted;
