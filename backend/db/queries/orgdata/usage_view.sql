-- name: ReadTermUsage :one
SELECT calls FROM org_data_usage_terms
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND term_start=sqlc.arg('term_start')::timestamptz
  AND term_end=sqlc.arg('term_end')::timestamptz;

-- name: ListAdmittedPlanDatasets :many
SELECT d.dataset_key,d.revision,d.display_name,d.state_code,d.minimum_stations,d.method_version
FROM enterprise_data_plan_datasets p
JOIN enterprise_regional_datasets d ON (d.dataset_key,d.revision)=(p.dataset_key,p.dataset_revision)
JOIN enterprise_regional_series b ON (b.dataset_key,b.dataset_revision)=(d.dataset_key,d.revision)
CROSS JOIN LATERAL (
    SELECT revision,status FROM enterprise_data_source_reviews
    WHERE source_code=d.registry_source AND source_digest=d.registry_digest
    ORDER BY revision DESC LIMIT 1
) registry
CROSS JOIN LATERAL (
    SELECT revision,status FROM enterprise_data_source_reviews
    WHERE source_code=d.price_source AND source_digest=d.price_digest
    ORDER BY revision DESC LIMIT 1
) prices
WHERE p.product_code=sqlc.arg('product_code')::text AND p.plan_code=sqlc.arg('plan_code')::text
  AND p.plan_revision=sqlc.arg('plan_revision')::bigint
  AND registry.revision=d.registry_review AND registry.status='APPROVED'
  AND prices.revision=d.price_review AND prices.status='APPROVED'
  AND (d.dataset_key COLLATE "C",d.revision)>(sqlc.arg('after_key')::text COLLATE "C",sqlc.arg('after_revision')::bigint)
ORDER BY d.dataset_key COLLATE "C",d.revision
LIMIT sqlc.arg('row_limit')::integer;
