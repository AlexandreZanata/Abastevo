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

-- name: EnsureState :exec
INSERT INTO cloud_state(account_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: GetState :one
SELECT * FROM cloud_state WHERE account_id=$1 FOR UPDATE;

-- name: GetCloudOperation :one
SELECT payload_hash,receipt FROM cloud_operations WHERE account_id=$1 AND operation_id=$2;

-- name: CloudAdmission :one
SELECT count(*)::bigint AS total,count(*) FILTER(WHERE created_at>clock_timestamp()-interval '1 hour')::bigint AS hourly
FROM cloud_operations WHERE account_id=$1;

-- name: SaveCloudOperation :exec
INSERT INTO cloud_operations(account_id,operation_id,payload_hash,receipt) VALUES($1,$2,$3,$4);

-- name: NextRevision :one
UPDATE cloud_state SET revision=revision+1 WHERE account_id=$1 RETURNING revision;

-- name: ErasedID :one
SELECT EXISTS(SELECT 1 FROM cloud_erased_ids WHERE account_id=$1 AND dataset=$2 AND id=$3);

-- name: EraseID :exec
INSERT INTO cloud_erased_ids(account_id,dataset,id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: DeleteRecord :one
UPDATE cloud_records SET payload=NULL,deleted=true,revision=revision+1,updated_at=clock_timestamp()
WHERE account_id=$1 AND dataset=$2 AND id=$3 AND NOT deleted RETURNING revision;

-- name: ClearChangePayloads :exec
UPDATE cloud_changes SET payload=NULL,deleted=true WHERE account_id=$1 AND dataset=$2 AND id=$3;

-- name: SaveChange :exec
INSERT INTO cloud_changes(account_id,sequence,epoch,dataset,id,record_revision,deleted,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: ListRecords :many
SELECT dataset,id,revision,deleted,payload FROM cloud_records
WHERE account_id=$1 AND dataset=$2 AND NOT deleted AND id>$3 ORDER BY id LIMIT $4;

-- name: ListDevices :many
SELECT id,origin,created_at,last_success_at,revoked_at FROM cloud_devices
WHERE account_id=$1 AND origin=$2 AND id>$3 ORDER BY id LIMIT $4;


-- name: NeedsErasure :one
SELECT EXISTS(SELECT 1 FROM cloud_records c WHERE c.account_id=sqlc.arg(account_id)::uuid AND NOT c.deleted
AND (sqlc.arg(dataset)::text='all' OR c.dataset=sqlc.arg(dataset)) AND (sqlc.narg(record_id)::uuid IS NULL OR c.id=sqlc.narg(record_id)))
OR (sqlc.arg(dataset)::text='all' AND EXISTS(SELECT 1 FROM cloud_devices d WHERE d.account_id=sqlc.arg(account_id) AND d.revoked_at IS NULL));

-- name: EraseSelectedIDs :exec
INSERT INTO cloud_erased_ids(account_id,dataset,id)
SELECT c.account_id,c.dataset,c.id FROM cloud_records c WHERE c.account_id=sqlc.arg(account_id)::uuid
AND (sqlc.arg(dataset)::text='all' OR c.dataset=sqlc.arg(dataset)) AND (sqlc.narg(record_id)::uuid IS NULL OR c.id=sqlc.narg(record_id))
ON CONFLICT DO NOTHING;

-- name: EraseSelectedRecords :exec
UPDATE cloud_records c SET deleted=true,payload=NULL,revision=revision+1,updated_at=clock_timestamp()
WHERE c.account_id=sqlc.arg(account_id)::uuid AND NOT c.deleted
AND (sqlc.arg(dataset)::text='all' OR c.dataset=sqlc.arg(dataset)) AND (sqlc.narg(record_id)::uuid IS NULL OR c.id=sqlc.narg(record_id));

-- name: EraseSelectedChanges :exec
UPDATE cloud_changes c SET deleted=true,payload=NULL WHERE c.account_id=sqlc.arg(account_id)::uuid
AND (sqlc.arg(dataset)::text='all' OR c.dataset=sqlc.arg(dataset)) AND (sqlc.narg(record_id)::uuid IS NULL OR c.id=sqlc.narg(record_id));

-- name: ErasureEpoch :one
UPDATE cloud_state SET epoch=epoch+1,revision=revision+1,consent_mask=consent_mask & (~sqlc.arg(mask)::integer),
consent_revision=consent_revision+1 WHERE account_id=sqlc.arg(account_id)::uuid RETURNING epoch;

-- name: RevokeAllCloudDevices :exec
WITH revoked AS(UPDATE cloud_devices d SET revoked_at=COALESCE(d.revoked_at,clock_timestamp()) WHERE d.account_id=$1 RETURNING d.id,d.account_id)
INSERT INTO cloud_device_revocations(device_id,account_id) SELECT id,account_id FROM revoked ON CONFLICT DO NOTHING;

-- name: ListChanges :many
SELECT sequence,epoch,dataset,id,record_revision,deleted,payload FROM cloud_changes
WHERE account_id=$1 AND sequence>$2 AND sequence<=$3 ORDER BY sequence LIMIT $4;

-- name: SetConsent :one
UPDATE cloud_state SET
 consent_mask=CASE WHEN sqlc.arg(enabled)::boolean THEN consent_mask | sqlc.arg(mask)::integer ELSE consent_mask & (~sqlc.arg(mask)::integer) END,
 consent_revision=consent_revision+1, revision=revision+1
WHERE account_id=sqlc.arg(account_id)::uuid RETURNING epoch,revision,consent_mask,consent_revision;

-- name: CompactFloor :one
SELECT COALESCE(MAX(sequence),0)::bigint FROM cloud_changes WHERE account_id=$1 AND created_at<$2;

-- name: DeleteOldChanges :execrows
DELETE FROM cloud_changes c WHERE c.ctid IN (SELECT c2.ctid FROM cloud_changes c2 WHERE c2.account_id=$1 AND c2.sequence<=$2 AND c2.created_at<$3 LIMIT 500);

-- name: DeleteOldReceipts :execrows
DELETE FROM cloud_operations o WHERE o.ctid IN (SELECT o2.ctid FROM cloud_operations o2 WHERE o2.account_id=$1 AND o2.created_at<$2 LIMIT 500);

-- name: AdvanceFloor :one
UPDATE cloud_state SET compacted_through=GREATEST(compacted_through,sqlc.arg(floor)::bigint) WHERE account_id=sqlc.arg(account_id)::uuid RETURNING compacted_through;

-- name: InsertSnapshot :one
INSERT INTO cloud_snapshots(account_id,snapshot_id,revision,epoch,consent_revision,byte_size,checksum,status,object_key)
VALUES(sqlc.arg(account_id)::uuid,sqlc.arg(snapshot_id)::uuid,sqlc.arg(revision)::bigint,sqlc.arg(epoch)::bigint,sqlc.arg(consent_revision)::bigint,sqlc.arg(byte_size)::bigint,sqlc.arg(checksum)::text,'PENDING',sqlc.arg(object_key)::text)
ON CONFLICT DO NOTHING RETURNING snapshot_id,revision,epoch,consent_revision,byte_size,checksum,status,object_key,created_at,verified_at;

-- name: GetSnapshot :one
SELECT snapshot_id,revision,epoch,consent_revision,byte_size,checksum,status,object_key,created_at,verified_at FROM cloud_snapshots WHERE account_id=$1 AND snapshot_id=$2;

-- name: ListReadySnapshots :many
SELECT snapshot_id,revision,epoch,consent_revision,byte_size,checksum,status,object_key,created_at,verified_at FROM cloud_snapshots WHERE account_id=$1 AND status='READY' ORDER BY revision DESC, created_at DESC LIMIT $2;

-- name: MarkSnapshotReady :execrows
UPDATE cloud_snapshots SET status='READY',verified_at=clock_timestamp() WHERE account_id=$1 AND snapshot_id=$2 AND status='PENDING' AND checksum=$3;

-- name: MarkSnapshotFailed :execrows
UPDATE cloud_snapshots SET status='FAILED' WHERE account_id=$1 AND snapshot_id=$2 AND status='PENDING';

-- name: DeleteSnapshotObject :exec
DELETE FROM cloud_snapshots WHERE account_id=$1 AND snapshot_id=$2 AND status='FAILED';

-- name: DueSnapshotAccounts :many
SELECT s.account_id FROM cloud_state s WHERE s.consent_mask!=0 AND ((SELECT COUNT(*) FROM cloud_snapshots c WHERE c.account_id=s.account_id AND c.status='READY')=0 OR (SELECT MAX(c.revision) FROM cloud_snapshots c WHERE c.account_id=s.account_id AND c.status='READY')<s.revision OR (SELECT MAX(c.created_at) FROM cloud_snapshots c WHERE c.account_id=s.account_id AND c.status='READY')<$1) ORDER BY s.account_id LIMIT $2;

-- name: DeleteSnapshot :exec
DELETE FROM cloud_snapshots WHERE account_id=$1 AND snapshot_id=$2 AND status='READY';

-- name: GetRestore :one
SELECT snapshot_id,pre_revision,post_revision,receipt FROM cloud_restores WHERE account_id=$1 AND restore_id=$2;

-- name: SaveRestore :exec
INSERT INTO cloud_restores(account_id,restore_id,snapshot_id,pre_revision,post_revision,receipt) VALUES($1,$2,$3,$4,$5,$6);
