-- Ownership joins are reviewed module ports: ledger checks vehicle ownership
-- via cloud_records for the same account without importing adapters.
-- name: LedgerVehicle :one
SELECT id FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2;

-- name: GetLedgerRecord :one
SELECT dataset,id,vehicle_id,occurred_at,revision,voided,deleted,payload FROM ledger_records
WHERE account_id=$1 AND dataset=$2 AND id=$3 FOR UPDATE;

-- name: CountLedgerDataset :one
SELECT count(*) FROM ledger_records WHERE account_id=$1 AND dataset=$2 AND NOT deleted;

-- name: SaveLedgerRecord :one
INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload)
VALUES($1,$2,$3,$4,$5,$6)
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
