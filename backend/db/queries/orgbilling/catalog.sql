-- name: GetPlanVersion :one
SELECT product_code, plan_code, revision, display_name, capabilities,
       active_vehicles, active_stations, storage_bytes, api_calls
FROM enterprise_plan_catalog
WHERE product_code = $1 AND plan_code = $2 AND revision = $3;
