-- name: ReadDatasetVersion :one
SELECT d.dataset_key,d.revision,d.display_name,d.state_code,d.minimum_stations,d.method_version,d.snapshot_digest,
       d.official_revision::text AS official_revision,d.registry_digest,d.registry_review,d.price_digest,d.price_review,
       registry.revision AS registry_current_revision,registry.status AS registry_status,
       registry.license_ref AS registry_license,registry.attribution AS registry_attribution,
       prices.revision AS price_current_revision,prices.status AS price_status,
       prices.license_ref AS price_license,prices.attribution AS price_attribution
FROM enterprise_regional_datasets d
CROSS JOIN LATERAL (
    SELECT r.revision,r.status,r.license_ref,r.attribution FROM enterprise_data_source_reviews r
    WHERE r.source_code=d.registry_source AND r.source_digest=d.registry_digest
    ORDER BY r.revision DESC LIMIT 1
) registry
CROSS JOIN LATERAL (
    SELECT r.revision,r.status,r.license_ref,r.attribution FROM enterprise_data_source_reviews r
    WHERE r.source_code=d.price_source AND r.source_digest=d.price_digest
    ORDER BY r.revision DESC LIMIT 1
) prices
WHERE d.dataset_key=sqlc.arg('dataset_key')::text AND d.revision=sqlc.arg('revision')::bigint;
