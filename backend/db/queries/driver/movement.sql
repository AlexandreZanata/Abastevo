-- DRV-08 private shifts, recovery gaps, receipts and aggregate distances.

-- name: CurrentDriverShift :one
SELECT * FROM driver_shifts WHERE account_id=$1 AND state<>'CLOSED' FOR UPDATE;

-- name: GetDriverShift :one
SELECT * FROM driver_shifts WHERE account_id=$1 AND id=$2 FOR UPDATE;

-- name: InsertDriverShift :exec
INSERT INTO driver_shifts(account_id,id,origin,vehicle_id,timezone,state,version,last_boot_id,last_elapsed_millis,last_utc_millis,active_millis,manually_reviewed_millis)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: UpdateDriverShift :execrows
UPDATE driver_shifts SET state=$4,version=$5,last_boot_id=$6,last_elapsed_millis=$7,last_utc_millis=$8,active_millis=$9,manually_reviewed_millis=$10,updated_at=now()
WHERE account_id=$1 AND id=$2 AND version=$3;

-- name: InsertDriverShiftGap :exec
INSERT INTO driver_shift_gaps(account_id,shift_id,id,reason,previous_boot_id,previous_elapsed_millis,previous_utc_millis,next_boot_id,next_elapsed_millis,next_utc_millis)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: ReviewDriverShiftGap :execrows
UPDATE driver_shift_gaps SET reviewed=true,reviewed_work_millis=$4
WHERE account_id=$1 AND shift_id=$2 AND id=$3 AND NOT reviewed;

-- name: DriverShiftCommand :one
SELECT * FROM driver_shift_commands WHERE account_id=$1 AND shift_id=$2 AND operation_id=$3;

-- name: SaveDriverShiftCommand :exec
INSERT INTO driver_shift_commands(account_id,shift_id,operation_id,fingerprint,accepted_version)
VALUES($1,$2,$3,$4,$5);

-- name: PruneDriverShiftCommands :exec
DELETE FROM driver_shift_commands WHERE account_id=$1 AND shift_id=$2 AND accepted_version<=$3;

-- name: GetDriverDistance :one
SELECT * FROM driver_distance_observations WHERE account_id=$1 AND id=$2;

-- name: DriverDistanceCommand :one
SELECT * FROM driver_distance_commands WHERE account_id=$1 AND operation_id=$2;

-- name: SaveDriverDistanceCommand :exec
INSERT INTO driver_distance_commands(account_id,operation_id,request_sha256,record_id)
VALUES($1,$2,$3,$4);

-- name: SaveDriverDistance :exec
INSERT INTO driver_distance_observations(account_id,id,origin,vehicle_id,segment,basis,from_at,to_at,timezone,metres,complete,has_gap,quality,quality_version,quality_note,start_odometer_metres,end_odometer_metres,content_sha256)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18);

-- name: DriverDistanceOverlap :one
SELECT EXISTS(
 SELECT 1 FROM driver_distance_observations
 WHERE account_id=sqlc.arg(account_id) AND vehicle_id=sqlc.arg(vehicle_id) AND basis=sqlc.arg(basis)
   AND id<>sqlc.arg(excluding_id)
   AND from_at<sqlc.arg(to_at) AND to_at>sqlc.arg(from_at)
)::boolean;

-- name: ListDriverDistances :many
SELECT * FROM driver_distance_observations
WHERE account_id=$1 AND vehicle_id=$2 AND from_at>=$3 AND from_at<$4
ORDER BY from_at,id LIMIT $5;

-- name: ListDriverShiftGaps :many
SELECT * FROM driver_shift_gaps WHERE account_id=$1 AND shift_id=$2 ORDER BY created_at,id;
