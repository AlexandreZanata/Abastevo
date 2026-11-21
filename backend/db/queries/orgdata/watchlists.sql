-- name: CreateWatch :execrows
INSERT INTO org_data_watchlists(org_id,id,membership_id,account_id,dataset_key,fuel_product,baseline_revision,last_revision,next_check)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('watch_id')::text::uuid,sqlc.arg('member')::text::uuid,sqlc.arg('account')::text::uuid,
       sqlc.arg('dataset_key')::text,sqlc.arg('fuel_product')::text,sqlc.arg('baseline_revision')::bigint,sqlc.arg('baseline_revision')::bigint,
       sqlc.arg('next_check')::bigint)
ON CONFLICT DO NOTHING;

-- name: ReadWatch :one
SELECT id::text AS id,membership_id::text AS member_id,account_id::text AS account_id,dataset_key,fuel_product,baseline_revision,last_revision,active,next_check
FROM org_data_watchlists
WHERE (org_id,id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('watch_id')::text::uuid)
FOR UPDATE;

-- name: WatchOwnerCurrent :one
SELECT EXISTS (SELECT 1 FROM org_memberships
WHERE (org_id,id,account_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('member')::text::uuid,sqlc.arg('account')::text::uuid)
AND state='ACTIVE')::boolean AS active;

-- name: ListWatches :many
SELECT id::text AS id,dataset_key,fuel_product,baseline_revision,last_revision,active
FROM org_data_watchlists
WHERE (org_id,membership_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('member')::text::uuid)
  AND (sqlc.arg('after_id')::text='' OR id>NULLIF(sqlc.arg('after_id')::text,'')::uuid)
ORDER BY id LIMIT sqlc.arg('row_limit')::integer;

-- name: DisableWatch :execrows
UPDATE org_data_watchlists SET active=false
WHERE (org_id,id,membership_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('watch_id')::text::uuid,sqlc.arg('member')::text::uuid)
  AND active;

-- name: AdvanceWatch :execrows
UPDATE org_data_watchlists SET last_revision=sqlc.arg('last_revision')::bigint,next_check=sqlc.arg('next_check')::bigint
WHERE (org_id,id,next_check)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('watch_id')::text::uuid,sqlc.arg('expected_check')::bigint)
  AND active;

-- name: LatestWatchRevision :one
SELECT p.dataset_revision
FROM enterprise_data_plan_datasets p
JOIN enterprise_regional_series s ON (s.dataset_key,s.dataset_revision)=(p.dataset_key,p.dataset_revision)
WHERE (p.product_code,p.plan_code,p.plan_revision)=(sqlc.arg('product_code')::text,sqlc.arg('plan_code')::text,sqlc.arg('plan_revision')::bigint)
  AND p.dataset_key=sqlc.arg('dataset_key')::text AND p.dataset_revision>sqlc.arg('last_revision')::bigint
-- A correction of an earlier week may carry a higher revision; compare the latest survey.
ORDER BY s.survey_end DESC,p.dataset_revision DESC LIMIT 1;
