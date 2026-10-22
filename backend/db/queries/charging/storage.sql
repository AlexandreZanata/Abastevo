-- Private staging ports for EV charging/reference batches, not public
-- reads. Callers stage verified batches atomically; per-batch census is
-- checked before any read. No Directory writes; station links stay null
-- until a reviewed linking unit populates them.

-- name: RecordChargingBatch :one
INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
VALUES(@id::uuid,@format,@profile_id,@source_revision,@manifest_sha256,@input_count::int,@accepted_count::int)
ON CONFLICT DO NOTHING RETURNING id;

-- name: StageChargingPlace :one
INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points)
VALUES(@id::uuid,@batch_id::uuid,@entity_key,@external_id,@name,@country,@latitude,@longitude,@public_access,@reported_points::int)
ON CONFLICT DO NOTHING RETURNING id;

-- name: StageChargingConnectionGroup :one
INSERT INTO charging_connection_groups(id,batch_id,place_id,entity_key,external_id,connector_code,raw_connector_code,current,power_w,reported_quantity)
VALUES(@id::uuid,@batch_id::uuid,@place_id::uuid,@entity_key,@external_id,@connector_code,@raw_connector_code,@current,sqlc.narg(power_w)::int,sqlc.narg(reported_quantity)::int)
ON CONFLICT DO NOTHING RETURNING id;

-- name: StageChargingVehicleReference :one
INSERT INTO charging_vehicle_references(id,batch_id,entity_key,model_code)
VALUES(@id::uuid,@batch_id::uuid,@entity_key,@model_code)
ON CONFLICT DO NOTHING RETURNING id;

-- name: ChargingBatchCounts :one
SELECT count(*) FILTER (WHERE kind = 'place') AS places,
 count(*) FILTER (WHERE kind = 'group') AS groups,
 count(*) FILTER (WHERE kind = 'vehicle') AS vehicles
FROM (
 SELECT 'place' AS kind FROM charging_places WHERE batch_id = @batch_id::uuid
 UNION ALL SELECT 'group' FROM charging_connection_groups WHERE batch_id = @batch_id::uuid
 UNION ALL SELECT 'vehicle' FROM charging_vehicle_references WHERE batch_id = @batch_id::uuid
) AS census;
