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

-- name: FindChargingBatchByManifest :one
SELECT id FROM charging_batches WHERE manifest_sha256 = @manifest_sha256 LIMIT 1;

-- name: ChargingPlacePage :many
SELECT p.id,p.entity_key,p.external_id,p.name,p.country,p.latitude,p.longitude,p.public_access,p.reported_points
FROM charging_places p
JOIN charging_batches b ON b.id = p.batch_id
WHERE b.profile_id = @profile::text
 AND (@revision::text = '' OR b.source_revision = @revision::text)
 AND (p.id::text > @after_id::text OR @after_id::text = '')
ORDER BY p.id::text LIMIT @limit_plus_one::int;

-- name: ChargingPlaceDetail :one
SELECT p.id,p.entity_key,p.external_id,p.name,p.country,p.latitude,p.longitude,p.public_access,p.reported_points,
 b.profile_id,b.source_revision
FROM charging_places p
JOIN charging_batches b ON b.id = p.batch_id
WHERE p.id = @id::uuid;

-- name: ChargingGroupsForPlace :many
SELECT id,entity_key,external_id,connector_code,raw_connector_code,current,power_w,reported_quantity
FROM charging_connection_groups WHERE place_id = @place_id::uuid ORDER BY entity_key;

-- name: ChargingBatchCoverage :one
SELECT b.profile_id,b.source_revision,b.input_count,b.accepted_count,
 (SELECT count(*) FROM charging_places WHERE batch_id = b.id) AS places,
 (SELECT count(*) FROM charging_connection_groups WHERE batch_id = b.id) AS groups,
 (SELECT count(*) FROM charging_vehicle_references WHERE batch_id = b.id) AS vehicles
FROM charging_batches b WHERE b.id = @batch_id::uuid;

-- name: LatestChargingBatch :one
SELECT id,profile_id,source_revision FROM charging_batches
WHERE profile_id = @profile::text ORDER BY stored_at DESC LIMIT 1;

-- name: FindChargingBatch :one
SELECT id FROM charging_batches WHERE profile_id = @profile::text AND (@revision::text = '' OR source_revision = @revision::text) ORDER BY stored_at DESC LIMIT 1;
