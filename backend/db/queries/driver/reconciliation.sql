-- DRV-10 owner-reviewed overlap decisions and exact operation receipts.

-- name: DriverReconciliationCommand :one
SELECT * FROM driver_reconciliation_commands WHERE account_id=$1 AND operation_id=$2;

-- name: SaveDriverReconciliationCommand :exec
INSERT INTO driver_reconciliation_commands(account_id,operation_id,request_sha256,decision_id,version) VALUES($1,$2,$3,$4,$5);

-- name: LatestDriverReconciliationVersion :one
SELECT COALESCE(MAX(version),0)::bigint FROM driver_reconciliation_decisions WHERE account_id=$1 AND id=$2;

-- name: SaveDriverReconciliation :exec
INSERT INTO driver_reconciliation_decisions(account_id,id,version,kind,platform,from_at,to_at,primary_id,primary_revision,payout_source_hash,content_sha256)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: SaveDriverReconciliationRecord :exec
INSERT INTO driver_reconciliation_records(account_id,decision_id,version,record_id,record_revision) VALUES($1,$2,$3,$4,$5);

-- name: GetDriverReconciliation :one
SELECT * FROM driver_reconciliation_decisions WHERE account_id=$1 AND id=$2 AND version=$3;

-- name: DriverReconciliationRecords :many
SELECT * FROM driver_reconciliation_records WHERE account_id=$1 AND decision_id=$2 AND version=$3 ORDER BY record_id;

-- name: ListDriverReconciliations :many
SELECT * FROM driver_reconciliation_decisions WHERE account_id=$1 AND id>$2 ORDER BY id,version LIMIT $3;

-- name: DeleteDriverOwnerReconciliations :exec
DELETE FROM driver_reconciliation_decisions WHERE account_id=$1;

-- name: DeleteDriverOwnerReconciliationCommands :exec
DELETE FROM driver_reconciliation_commands WHERE account_id=$1;
