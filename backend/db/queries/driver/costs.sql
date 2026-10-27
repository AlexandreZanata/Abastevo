-- DRV-09 cost profiles, maintenance schedules and erasure fences.

-- name: EnsureDriverCostState :exec
INSERT INTO driver_cost_owner_state(account_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: LockDriverCostEpoch :one
SELECT epoch FROM driver_cost_owner_state WHERE account_id=$1 FOR UPDATE;

-- name: AdvanceDriverCostEpoch :one
UPDATE driver_cost_owner_state SET epoch=epoch+1 WHERE account_id=$1 RETURNING epoch;

-- name: DriverCostTombstone :one
SELECT * FROM driver_cost_tombstones WHERE account_id=$1 AND vehicle_id=$2 AND version=$3;

-- name: SaveDriverCostTombstone :exec
INSERT INTO driver_cost_tombstones(account_id,vehicle_id,version,erased_content_sha256) VALUES($1,$2,$3,$4);

-- name: DriverCostCommand :one
SELECT * FROM driver_cost_commands WHERE account_id=$1 AND operation_id=$2;

-- name: SaveDriverCostCommand :exec
INSERT INTO driver_cost_commands(account_id,operation_id,request_sha256,vehicle_id,version) VALUES($1,$2,$3,$4,$5);

-- name: LatestDriverCostVersion :one
SELECT COALESCE(MAX(version),0)::bigint FROM driver_cost_profiles WHERE account_id=$1 AND vehicle_id=$2;

-- name: SaveDriverCostProfile :exec
INSERT INTO driver_cost_profiles(account_id,vehicle_id,version,ownership,timezone,from_day,until_day,content_sha256)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: SaveDriverCostItem :exec
INSERT INTO driver_cost_profile_items(account_id,vehicle_id,version,cost_id,kind,provenance,ledger_source_id,source_revision,from_day,until_day,amount_centavos)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: GetDriverCostProfile :one
SELECT * FROM driver_cost_profiles WHERE account_id=$1 AND vehicle_id=$2 AND version=$3;

-- name: DriverCostItems :many
SELECT * FROM driver_cost_profile_items WHERE account_id=$1 AND vehicle_id=$2 AND version=$3 ORDER BY cost_id;

-- name: ListDriverCostProfiles :many
SELECT * FROM driver_cost_profiles WHERE account_id=$1 AND vehicle_id=$2 AND version>$3 ORDER BY version LIMIT $4;

-- name: DeleteDriverCostProfile :exec
DELETE FROM driver_cost_profiles WHERE account_id=$1 AND vehicle_id=$2 AND version=$3;

-- name: DeleteDriverCostOwnerProfiles :exec
DELETE FROM driver_cost_profiles WHERE account_id=$1;

-- name: DeleteDriverCostOwnerCommands :exec
DELETE FROM driver_cost_commands WHERE account_id=$1;

-- name: DeleteDriverCostOwnerTombstones :exec
DELETE FROM driver_cost_tombstones WHERE account_id=$1;

-- name: EnsureDriverMaintenanceState :exec
INSERT INTO driver_maintenance_owner_state(account_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: LockDriverMaintenanceEpoch :one
SELECT epoch FROM driver_maintenance_owner_state WHERE account_id=$1 FOR UPDATE;

-- name: AdvanceDriverMaintenanceEpoch :one
UPDATE driver_maintenance_owner_state SET epoch=epoch+1 WHERE account_id=$1 RETURNING epoch;

-- name: DriverMaintenanceTombstone :one
SELECT * FROM driver_maintenance_tombstones WHERE account_id=$1 AND schedule_id=$2 AND version=$3;

-- name: SaveDriverMaintenanceTombstone :exec
INSERT INTO driver_maintenance_tombstones(account_id,schedule_id,version,erased_content_sha256) VALUES($1,$2,$3,$4);

-- name: DriverMaintenanceCommand :one
SELECT * FROM driver_maintenance_commands WHERE account_id=$1 AND operation_id=$2;

-- name: SaveDriverMaintenanceCommand :exec
INSERT INTO driver_maintenance_commands(account_id,operation_id,request_sha256,schedule_id,version) VALUES($1,$2,$3,$4,$5);

-- name: LatestDriverMaintenanceVersion :one
SELECT COALESCE(MAX(version),0)::bigint FROM driver_maintenance_schedules WHERE account_id=$1 AND id=$2;

-- name: SaveDriverMaintenance :exec
INSERT INTO driver_maintenance_schedules(account_id,id,version,vehicle_id,timezone,kind,basis,effective_from,effective_until,interval_metres,interval_days,odometer_segment,baseline_metres,baseline_day,next_due_metres,next_due_day,content_sha256)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17);

-- name: GetDriverMaintenance :one
SELECT * FROM driver_maintenance_schedules WHERE account_id=$1 AND id=$2 AND version=$3;

-- name: ListDriverMaintenance :many
SELECT * FROM driver_maintenance_schedules WHERE account_id=$1 AND vehicle_id=$2 AND version>$3 ORDER BY version LIMIT $4;

-- name: DeleteDriverMaintenance :exec
DELETE FROM driver_maintenance_schedules WHERE account_id=$1 AND id=$2 AND version=$3;

-- name: DeleteDriverMaintenanceOwnerSchedules :exec
DELETE FROM driver_maintenance_schedules WHERE account_id=$1;

-- name: DeleteDriverMaintenanceOwnerCommands :exec
DELETE FROM driver_maintenance_commands WHERE account_id=$1;

-- name: DeleteDriverMaintenanceOwnerTombstones :exec
DELETE FROM driver_maintenance_tombstones WHERE account_id=$1;
