-- DRV-11 revisioned report snapshots and bounded retention cleanup.

-- name: UpsertDriverReportSnapshot :one
INSERT INTO driver_report_snapshots(account_id,id,vehicle_id,period,from_day,to_day,timezone,platform,basis,source_revision,payload)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(account_id,vehicle_id,period,from_day,to_day,timezone,platform,basis)
DO UPDATE SET source_revision=EXCLUDED.source_revision,payload=EXCLUDED.payload,version=driver_report_snapshots.version+1,generated_at=now()
RETURNING version,generated_at;

-- name: GetDriverReportSnapshot :one
SELECT * FROM driver_report_snapshots
WHERE account_id=$1 AND vehicle_id=$2 AND period=$3 AND from_day=$4 AND to_day=$5 AND timezone=$6 AND platform=$7 AND basis=$8;

-- name: DeleteExpiredDriverReportSnapshots :execrows
DELETE FROM driver_report_snapshots d WHERE (d.account_id,d.id) IN (
 SELECT s.account_id,s.id FROM driver_report_snapshots s WHERE s.generated_at<$1 ORDER BY s.generated_at LIMIT $2
);
