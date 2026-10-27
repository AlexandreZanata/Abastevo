-- Transaction-bound port owned by cloudsync/account storage, DRV-07.
-- name: LockDriverActiveAccount :one
SELECT id FROM accounts WHERE id=$1 AND status='active' FOR UPDATE;
-- name: LockDriverOwnedVehicle :one
SELECT id FROM cloud_records WHERE account_id=$1 AND id=$2 AND dataset='vehicles' AND NOT deleted FOR UPDATE;
