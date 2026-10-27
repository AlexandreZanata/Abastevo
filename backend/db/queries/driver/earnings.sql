-- name: EnsureDriverOwnerState :exec
INSERT INTO driver_earning_owner_state(account_id) VALUES($1) ON CONFLICT DO NOTHING;
-- name: LockDriverEpoch :one
SELECT epoch FROM driver_earning_owner_state WHERE account_id=$1 FOR UPDATE;
-- name: DriverCommand :one
SELECT * FROM driver_earning_commands WHERE account_id=$1 AND operation_id=$2;
-- name: SaveDriverCommand :exec
INSERT INTO driver_earning_commands(account_id,operation_id,request_sha256,record_id,revision,epoch) VALUES($1,$2,$3,$4,$5,$6);
-- name: GetDriverEarning :one
SELECT * FROM driver_earnings WHERE account_id=$1 AND id=$2;
-- name: FindDriverSource :one
SELECT * FROM driver_earnings WHERE account_id=$1 AND platform=$2 AND source_hash=$3;
-- name: DriverCoverageConflict :one
SELECT EXISTS(SELECT 1 FROM driver_earnings WHERE account_id=sqlc.arg(account_id) AND platform=sqlc.arg(platform) AND id<>sqlc.arg(excluding_id)
 AND from_at<sqlc.arg(to_at) AND to_at>sqlc.arg(from_at))::boolean;
-- name: SaveDriverEarning :one
INSERT INTO driver_earnings(account_id,id,vehicle_id,platform,source_hash,coverage_kind,currency,from_at,to_at,timezone,basis,gross_centavos,fees_centavos,tips_centavos,bonus_centavos,refund_centavos,net_centavos,content_sha256)
VALUES(sqlc.arg(account_id),sqlc.arg(id),sqlc.arg(vehicle_id),sqlc.arg(platform),sqlc.arg(source_hash),sqlc.arg(coverage_kind),sqlc.arg(currency),sqlc.arg(from_at),sqlc.arg(to_at),sqlc.arg(timezone),sqlc.arg(basis),sqlc.narg(gross_centavos),sqlc.narg(fees_centavos),sqlc.narg(tips_centavos),sqlc.narg(bonus_centavos),sqlc.narg(refund_centavos),sqlc.narg(net_centavos),sqlc.arg(content_sha256))
ON CONFLICT(account_id,id) DO UPDATE SET vehicle_id=EXCLUDED.vehicle_id,platform=EXCLUDED.platform,source_hash=EXCLUDED.source_hash,coverage_kind=EXCLUDED.coverage_kind,currency=EXCLUDED.currency,from_at=EXCLUDED.from_at,to_at=EXCLUDED.to_at,timezone=EXCLUDED.timezone,basis=EXCLUDED.basis,gross_centavos=EXCLUDED.gross_centavos,fees_centavos=EXCLUDED.fees_centavos,tips_centavos=EXCLUDED.tips_centavos,bonus_centavos=EXCLUDED.bonus_centavos,refund_centavos=EXCLUDED.refund_centavos,net_centavos=EXCLUDED.net_centavos,content_sha256=EXCLUDED.content_sha256,revision=driver_earnings.revision+1
WHERE driver_earnings.revision=sqlc.arg(expected_revision) RETURNING *;
-- name: SaveDriverRevision :exec
INSERT INTO driver_earning_revisions(account_id,record_id,revision,payload) VALUES($1,$2,$3,$4);
-- name: DriverRevisions :many
SELECT * FROM driver_earning_revisions WHERE account_id=$1 AND record_id=$2 ORDER BY revision;
-- name: DriverTombstone :one
SELECT * FROM driver_earning_tombstones WHERE account_id=$1 AND record_id=$2;
-- name: DriverSourceErased :one
SELECT EXISTS(SELECT 1 FROM driver_earning_tombstones WHERE account_id=$1 AND platform=$2 AND source_hash=$3)::boolean;
-- name: ListDriverEarnings :many
SELECT * FROM driver_earnings WHERE account_id=$1 AND id>$2 ORDER BY id LIMIT $3;
-- name: DriverAdjustments :many
SELECT * FROM driver_earning_adjustments WHERE account_id=$1 AND target_id=$2 ORDER BY id;
-- name: DriverAdjustmentSource :one
SELECT * FROM driver_earning_adjustments WHERE account_id=$1 AND source_hash=$2;
-- name: SaveDriverAdjustment :exec
INSERT INTO driver_earning_adjustments(account_id,id,source_hash,target_id,target_revision,kind,delta_centavos,content_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8);
-- name: TombstoneDriverEarning :exec
INSERT INTO driver_earning_tombstones(account_id,record_id,platform,source_hash,erased_revision) VALUES($1,$2,$3,$4,$5);
-- name: DeleteDriverEarning :exec
DELETE FROM driver_earnings WHERE account_id=$1 AND id=$2;
-- name: DeleteDriverOwnerEarnings :exec
DELETE FROM driver_earnings WHERE account_id=$1;
-- name: DeleteDriverOwnerCommands :exec
DELETE FROM driver_earning_commands WHERE account_id=$1;
-- name: DeleteDriverOwnerTombstones :exec
DELETE FROM driver_earning_tombstones WHERE account_id=$1;
-- name: AdvanceDriverEpoch :one
UPDATE driver_earning_owner_state SET epoch=epoch+1 WHERE account_id=$1 RETURNING epoch;
-- name: DriverAdjustmentStats :many
SELECT a.target_id, count(*)::bigint AS adjustment_count, bool_or(a.target_revision<>e.revision)::boolean AS needs_reconciliation
FROM driver_earning_adjustments a JOIN driver_earnings e ON e.account_id=a.account_id AND e.id=a.target_id
WHERE a.account_id=$1 AND a.target_id=ANY(sqlc.arg(target_ids)::uuid[]) GROUP BY a.target_id;
-- name: PageDriverAdjustments :many
SELECT * FROM driver_earning_adjustments WHERE account_id=$1 AND target_id=$2 AND id>$3 ORDER BY id LIMIT $4;
-- name: LockDriverOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(owner_key)::text,607));
-- name: PageDriverRevisions :many
SELECT * FROM driver_earning_revisions WHERE account_id=$1 AND record_id=$2 AND revision>$3 ORDER BY revision LIMIT $4;
