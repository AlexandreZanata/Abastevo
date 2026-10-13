-- Ownership joins are reviewed module ports: ledger checks vehicle ownership
-- via cloud_records for the same account without importing adapters.
-- name: LedgerVehicle :one
SELECT id FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2;

-- name: GetLedgerRecord :one
SELECT dataset,id,vehicle_id,occurred_at,revision,voided,deleted,payload,segment_no FROM ledger_records
WHERE account_id=$1 AND dataset=$2 AND id=$3 FOR UPDATE;

-- name: CountLedgerDataset :one
SELECT count(*) FROM ledger_records WHERE account_id=$1 AND dataset=$2 AND NOT deleted;

-- name: SaveLedgerRecord :one
INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload,segment_no)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(account_id,dataset,id) DO UPDATE SET vehicle_id=EXCLUDED.vehicle_id,occurred_at=EXCLUDED.occurred_at,payload=EXCLUDED.payload,revision=ledger_records.revision+1,updated_at=clock_timestamp()
WHERE NOT ledger_records.deleted
RETURNING dataset,id,revision,voided,deleted,payload;

-- name: VoidLedgerRecord :one
UPDATE ledger_records SET voided=true,revision=revision+1,updated_at=clock_timestamp()
WHERE account_id=$1 AND dataset=$2 AND id=$3 AND NOT deleted AND NOT voided RETURNING revision;

-- name: DeleteLedgerRecord :one
UPDATE ledger_records SET payload='{}',deleted=true,revision=revision+1,updated_at=clock_timestamp()
WHERE account_id=$1 AND dataset=$2 AND id=$3 AND NOT deleted RETURNING revision;

-- name: GetLedgerOperation :one
SELECT payload_hash,receipt FROM ledger_operations WHERE account_id=$1 AND operation_id=$2;

-- name: SaveLedgerOperation :exec
INSERT INTO ledger_operations(account_id,operation_id,payload_hash,receipt) VALUES($1,$2,$3,$4);

-- name: MaxSegmentNo :one
SELECT COALESCE(MAX(segment_no),1)::bigint FROM ledger_segments WHERE account_id=$1 AND vehicle_id=$2;

-- name: OpenSegment :one
INSERT INTO ledger_segments(account_id,vehicle_id,segment_no,reason) VALUES($1,$2,$3,$4) RETURNING segment_no;

-- name: MaxSegmentOdometer :one
SELECT COALESCE(MAX((payload->>'odometer_metres')::bigint),-1)::bigint FROM ledger_records WHERE account_id=$1 AND vehicle_id=$2 AND segment_no=$3 AND NOT deleted AND payload->>'odometer_metres' IS NOT NULL;

-- name: MaxSegmentOdometerExcluding :one
SELECT COALESCE(MAX((payload->>'odometer_metres')::bigint),-1)::bigint FROM ledger_records WHERE account_id=$1 AND vehicle_id=$2 AND segment_no=$3 AND id!=$4 AND NOT deleted AND payload->>'odometer_metres' IS NOT NULL;

-- name: DeclareGap :one
INSERT INTO ledger_gaps(account_id,gap_id,vehicle_id,after_id,before_id,note) VALUES($1,$2,$3,$4,$5,$6) RETURNING gap_id;
