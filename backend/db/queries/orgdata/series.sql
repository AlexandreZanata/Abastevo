-- name: CheckPlanDataset :one
SELECT EXISTS(
    SELECT 1 FROM enterprise_data_plan_datasets
    WHERE product_code=sqlc.arg('product_code')::text AND plan_code=sqlc.arg('plan_code')::text
      AND plan_revision=sqlc.arg('plan_revision')::bigint AND dataset_key=sqlc.arg('dataset_key')::text
      AND dataset_revision=sqlc.arg('dataset_revision')::bigint
)::boolean AS allowed;

-- name: ReadSeriesBasis :one
SELECT survey_start,survey_end,published_at,source_url,result_sha256 FROM enterprise_regional_series
WHERE dataset_key=sqlc.arg('dataset_key')::text AND dataset_revision=sqlc.arg('dataset_revision')::bigint;

-- name: ListSeriesSamples :many
SELECT fuel_product,unit_code,stations,observations,sum_milli_brl,minimum_milli_brl,maximum_milli_brl
FROM enterprise_regional_series_samples
WHERE dataset_key=sqlc.arg('dataset_key')::text AND dataset_revision=sqlc.arg('dataset_revision')::bigint
ORDER BY fuel_product COLLATE "C";
