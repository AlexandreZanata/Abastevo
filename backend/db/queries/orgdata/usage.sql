-- name: LockDataUsage :exec
SELECT pg_catalog.pg_advisory_xact_lock(
    pg_catalog.hashtextextended('org-data-usage:' || sqlc.arg('org_id')::text,0)
);

-- name: ResolveKeyFamily :one
WITH RECURSIVE ancestors(id) AS (
    SELECT CAST(sqlc.arg('key_id')::text AS uuid)
    UNION
    SELECT parents.id FROM org_data_api_keys parents
    JOIN ancestors ON parents.replacement_id=ancestors.id
    WHERE parents.org_id=CAST(sqlc.arg('org_id')::text AS uuid)
)
SELECT ancestors.id::text AS family_id FROM ancestors
WHERE NOT EXISTS (
    SELECT 1 FROM org_data_api_keys parent
    WHERE parent.org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND parent.replacement_id=ancestors.id
);

-- name: ReadRatePolicy :one
SELECT window_seconds,window_calls FROM enterprise_data_rate_policies
WHERE product_code=sqlc.arg('product_code') AND plan_code=sqlc.arg('plan_code') AND plan_revision=sqlc.arg('plan_revision');

-- name: ReadDataReceipt :one
SELECT request_hash FROM org_data_request_receipts
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid)
  AND family_id=CAST(sqlc.arg('family_id')::text AS uuid)
  AND request_id=CAST(sqlc.arg('request_id')::text AS uuid);

-- name: AddDataCallToWindow :execrows
INSERT INTO org_data_rate_windows (org_id,family_id,window_start,window_seconds,calls)
SELECT CAST(sqlc.arg('org_id')::text AS uuid),CAST(sqlc.arg('family_id')::text AS uuid),
       sqlc.arg('window_start')::timestamptz,sqlc.arg('window_seconds')::integer,1
WHERE sqlc.arg('ceiling')::bigint > 0
ON CONFLICT (org_id,family_id,window_start,window_seconds) DO UPDATE
SET calls=org_data_rate_windows.calls+1
WHERE org_data_rate_windows.calls < sqlc.arg('ceiling')::bigint;

-- name: AddDataCallToTerm :execrows
INSERT INTO org_data_usage_terms (org_id,term_start,term_end,calls)
SELECT CAST(sqlc.arg('org_id')::text AS uuid),sqlc.arg('term_start')::timestamptz,sqlc.arg('term_end')::timestamptz,1
WHERE sqlc.arg('ceiling')::bigint > 0
ON CONFLICT (org_id,term_start) DO UPDATE
SET calls=org_data_usage_terms.calls+1
WHERE org_data_usage_terms.term_end=EXCLUDED.term_end AND org_data_usage_terms.calls < sqlc.arg('ceiling')::bigint;

-- name: RecordDataReceipt :exec
INSERT INTO org_data_request_receipts
    (org_id,family_id,request_id,key_id,term_start,accepted_at,request_hash)
VALUES(CAST(sqlc.arg('org_id')::text AS uuid),CAST(sqlc.arg('family_id')::text AS uuid),
       CAST(sqlc.arg('request_id')::text AS uuid),CAST(sqlc.arg('key_id')::text AS uuid),
       sqlc.arg('term_start')::timestamptz,sqlc.arg('accepted_at')::timestamptz,sqlc.arg('request_hash')::text);
