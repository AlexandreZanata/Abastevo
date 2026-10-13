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
