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

-- name: PurgeLedgerOperations :exec
DELETE FROM ledger_operations WHERE account_id=$1;

-- name: LockReport :one
SELECT pg_advisory_xact_lock(hashtext($1))::text;

-- name: BaseLedgerUpdated :one
SELECT MAX(changed_at)::timestamptz FROM (
 SELECT lr.updated_at AS changed_at FROM ledger_records lr WHERE lr.account_id=$1
 UNION ALL SELECT lg.created_at FROM ledger_gaps lg WHERE lg.account_id=$1
 UNION ALL SELECT ls.created_at FROM ledger_segments ls WHERE ls.account_id=$1
) AS changes;

-- name: ReportRows :many
SELECT r.dataset,r.id,r.vehicle_id,r.occurred_at,r.payload,r.segment_no,
 EXISTS (SELECT 1 FROM ledger_gaps g
 LEFT JOIN ledger_records a ON a.account_id=g.account_id AND a.dataset IN ('refuel','charge') AND a.id=g.after_id
 LEFT JOIN ledger_records b ON b.account_id=g.account_id AND b.dataset IN ('refuel','charge') AND b.id=g.before_id
 WHERE g.account_id=r.account_id AND g.vehicle_id=r.vehicle_id
 AND (a.occurred_at IS NULL OR a.occurred_at<$3)
 AND (b.occurred_at IS NULL OR b.occurred_at>=$2)) AS has_gap
FROM ledger_records r
WHERE r.account_id=$1 AND NOT r.deleted AND NOT r.voided AND r.occurred_at>=$2 AND r.occurred_at<$3 ORDER BY r.occurred_at,r.id;

-- name: GetReport :one
SELECT totals,revision,base_updated_at,generated_at FROM ledger_reports WHERE account_id=$1 AND year=$2 AND month=$3 AND timezone=$4;

-- name: UpsertReport :one
INSERT INTO ledger_reports(account_id,year,month,timezone,totals,base_updated_at)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(account_id,year,month,timezone) DO UPDATE SET totals=EXCLUDED.totals,revision=ledger_reports.revision+1,base_updated_at=EXCLUDED.base_updated_at,generated_at=clock_timestamp()
RETURNING totals,revision,base_updated_at,generated_at;

-- name: EnsureLedgerConsent :exec
INSERT INTO ledger_consent(account_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: GetLedgerConsent :one
SELECT mask,revision FROM ledger_consent WHERE account_id=$1;

-- name: SetLedgerConsent :one
UPDATE ledger_consent SET mask=CASE WHEN sqlc.arg(enabled)::boolean THEN mask | sqlc.arg(mask)::integer ELSE mask & (~sqlc.arg(mask)::integer) END, revision=revision+1, updated_at=clock_timestamp() WHERE account_id=sqlc.arg(account_id)::uuid RETURNING mask,revision;

-- name: EraseLedgerRecords :exec
UPDATE ledger_records SET payload='{}',deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=$1 AND NOT deleted;

-- name: ClearLedgerGaps :exec
DELETE FROM ledger_gaps WHERE account_id=$1;

-- name: ClearLedgerSegments :exec
DELETE FROM ledger_segments WHERE account_id=$1;

-- name: ListLedger :many
SELECT dataset,id,vehicle_id,occurred_at,revision,voided,deleted,payload FROM ledger_records
WHERE account_id=$1 AND dataset=$2 AND NOT deleted
AND (sqlc.narg(vehicle_id)::uuid IS NULL OR vehicle_id=sqlc.narg(vehicle_id)::uuid)
AND (sqlc.narg(category)::text IS NULL OR payload->>'category'=sqlc.narg(category)::text)
AND (sqlc.narg(from_time)::timestamptz IS NULL OR occurred_at>=sqlc.narg(from_time)::timestamptz)
AND (sqlc.narg(to_time)::timestamptz IS NULL OR occurred_at<sqlc.narg(to_time)::timestamptz)
AND (sqlc.narg(after_time)::timestamptz IS NULL OR occurred_at>sqlc.narg(after_time)::timestamptz OR (occurred_at=sqlc.narg(after_time)::timestamptz AND id>sqlc.narg(after_id)::uuid))
ORDER BY occurred_at,id LIMIT $3;

-- name: LockLedgerOwner :one
SELECT revision FROM ledger_consent WHERE account_id=$1 FOR UPDATE;

-- name: ClearLedgerReports :exec
DELETE FROM ledger_reports WHERE account_id=$1;

-- name: MetricRows :many
SELECT r.id,r.vehicle_id,r.payload,r.segment_no,
 EXISTS (SELECT 1 FROM ledger_gaps g
 LEFT JOIN ledger_records a ON a.account_id=g.account_id AND a.dataset IN ('refuel','charge') AND a.id=g.after_id
 LEFT JOIN ledger_records b ON b.account_id=g.account_id AND b.dataset IN ('refuel','charge') AND b.id=g.before_id
 WHERE g.account_id=r.account_id AND g.vehicle_id=r.vehicle_id
 AND (a.occurred_at IS NULL OR a.occurred_at<sqlc.arg(to_time)::timestamptz)
 AND (b.occurred_at IS NULL OR b.occurred_at>=sqlc.arg(from_time)::timestamptz)) AS has_gap
FROM ledger_records r
WHERE r.account_id=sqlc.arg(account_id)::uuid AND r.vehicle_id=sqlc.arg(vehicle_id)::uuid AND r.dataset='refuel'
AND NOT r.deleted AND NOT r.voided
AND r.occurred_at>=sqlc.arg(from_time)::timestamptz AND r.occurred_at<sqlc.arg(to_time)::timestamptz
ORDER BY r.occurred_at,r.id LIMIT sqlc.arg(row_limit)::int;

-- name: LedgerFingerprint :one
SELECT encode(sha256(convert_to(COALESCE(string_agg(token, ',' ORDER BY token), ''), 'UTF8')), 'hex')::text
FROM (
 SELECT 'r|'||r.dataset||'|'||r.id::text||'|'||r.revision::text AS token FROM ledger_records r WHERE r.account_id=$1
 UNION ALL SELECT 'g|'||g.gap_id::text FROM ledger_gaps g WHERE g.account_id=$1
 UNION ALL SELECT 's|'||s.vehicle_id::text||'|'||s.segment_no::text FROM ledger_segments s WHERE s.account_id=$1
) AS versions;

-- name: LockDriverLedgerCosts :many
SELECT dataset,id,vehicle_id,revision,voided,deleted,
 COALESCE(CASE WHEN dataset='expense' THEN payload->>'amount_centavos'
      ELSE payload->>'total_centavos' END,'')::text AS amount_text
FROM ledger_records WHERE account_id=$1 AND id=$2 FOR SHARE;
