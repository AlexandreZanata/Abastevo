-- Ownership joins are reviewed module ports: never import account/identity adapters.
-- name: LockOwner :one
SELECT id FROM accounts WHERE id=sqlc.arg(account_id)::uuid AND status='active' FOR UPDATE;

-- name: LockSession :one
SELECT id FROM account_session_families
WHERE id=sqlc.arg(family_id)::uuid AND account_id=sqlc.arg(account_id)::uuid
AND revoked_at IS NULL AND access_expires > clock_timestamp() FOR SHARE;

-- name: LockBinding :one
SELECT contributor_id FROM account_contributor_bindings
WHERE account_id=sqlc.arg(account_id)::uuid AND contributor_id=sqlc.arg(contributor_id)::text FOR SHARE;

-- name: LockSigningKey :one
SELECT k.id FROM identity_keys k JOIN identity_contributors c ON c.id=k.contributor_id
WHERE k.id=sqlc.arg(key_id)::uuid AND c.id=sqlc.arg(contributor_id)::uuid
AND k.revoked_at IS NULL AND c.status='active' FOR SHARE OF k,c;


-- name: GetRecord :one
SELECT dataset,id,revision,deleted,payload FROM cloud_records
WHERE account_id=$1 AND dataset=$2 AND id=$3 FOR UPDATE;

-- name: CountVehicles :one
SELECT count(*) FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND NOT deleted;

-- name: SaveRecord :one
INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,$2,$3,$4)
ON CONFLICT(account_id,dataset,id) DO UPDATE SET payload=EXCLUDED.payload,revision=cloud_records.revision+1,updated_at=clock_timestamp()
WHERE NOT cloud_records.deleted
RETURNING dataset,id,revision,deleted,payload;

-- name: CountDataset :one
SELECT count(*) FROM cloud_records WHERE account_id=$1 AND dataset=$2 AND NOT deleted;

-- name: ExistingDevice :one
SELECT d.id,d.created_at,d.last_success_at,d.revoked_at,d.origin FROM cloud_devices d
WHERE d.account_id=$1 AND d.family_id=$2 AND d.key_id=$3 AND d.origin=$4 AND d.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM cloud_device_revocations r WHERE r.device_id=d.id);

-- name: CountDevices :one
SELECT count(*) FROM cloud_devices WHERE account_id=$1 AND revoked_at IS NULL;

-- name: RegisterDevice :one
INSERT INTO cloud_devices(id,account_id,family_id,contributor_id,key_id,origin) VALUES($1,$2,$3,$4,$5,$6)
RETURNING id,created_at,last_success_at,revoked_at,origin;

-- name: LockDevice :one
SELECT d.id FROM cloud_devices d WHERE d.id=$1 AND d.account_id=$2 AND d.family_id=$3 AND d.contributor_id=$4 AND d.key_id=$5 AND d.origin=$6 AND d.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM cloud_device_revocations r WHERE r.device_id=d.id) FOR SHARE;

-- name: RevokeDevice :execrows
UPDATE cloud_devices SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE id=$1 AND account_id=$2 AND origin=$3;

-- name: DeviceSuccess :execrows
UPDATE cloud_devices SET last_success_at=clock_timestamp() WHERE id=$1 AND account_id=$2 AND origin=$3 AND revoked_at IS NULL;

-- name: FenceRestoredDevices :exec
UPDATE cloud_devices d SET revoked_at=clock_timestamp() WHERE d.account_id=$1 AND d.revoked_at IS NULL
AND EXISTS(SELECT 1 FROM cloud_device_revocations r WHERE r.device_id=d.id);

-- name: RecordDeviceRevocation :exec
INSERT INTO cloud_device_revocations(device_id,account_id)
SELECT d.id,d.account_id FROM cloud_devices d WHERE d.id=$1 AND d.account_id=$2 AND d.origin=$3 ON CONFLICT DO NOTHING;
