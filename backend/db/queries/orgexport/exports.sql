-- name: ExportRequesterLock :exec
-- Serializes requests of one requester: the open-job limit and replay are
-- decided under it (B-BR-EMP-227).
SELECT pg_advisory_xact_lock(hashtextextended('orgexport.requester:' || sqlc.arg(org_id)::text || ':' || sqlc.arg(membership_id)::text, 0));

-- name: ExportByOperation :one
SELECT CAST(id AS text) AS id, CAST(requester_account_id AS text) AS requester_account_id, CAST(requester_membership_id AS text) AS requester_membership_id, request_hash
FROM org_export_jobs WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND operation_id= CAST(sqlc.arg(operation_id)::text AS UUID);

-- name: CountOpenExports :one
SELECT count(*)::int AS open FROM org_export_jobs
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND requester_membership_id= CAST(sqlc.arg(membership_id)::text AS UUID) AND state IN('QUEUED','RUNNING');

-- name: InsertExport :exec
INSERT INTO org_export_jobs(org_id, id, operation_id, requester_account_id, requester_membership_id, kind, parameters, parameters_digest, request_hash, media_type,
 row_limit, byte_limit, time_limit_ms, retention_seconds, max_attempts, state, attempts, requested_at, as_of, updated_at)
VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(id)::text AS UUID), CAST(sqlc.arg(operation_id)::text AS UUID), CAST(sqlc.arg(requester_account_id)::text AS UUID),
 CAST(sqlc.arg(requester_membership_id)::text AS UUID), sqlc.arg(kind)::text, sqlc.arg(parameters)::text, sqlc.arg(parameters_digest)::text, sqlc.arg(request_hash)::text,
 sqlc.arg(media_type)::text, sqlc.arg(row_limit)::bigint, sqlc.arg(byte_limit)::bigint, sqlc.arg(time_limit_ms)::int, sqlc.arg(retention_seconds)::int,
 sqlc.arg(max_attempts)::smallint, 'QUEUED', 0, clock_timestamp(), clock_timestamp(), clock_timestamp());

-- name: ExportForRequester :one
SELECT CAST(id AS text) AS id, kind, parameters, parameters_digest, state, attempts, max_attempts, COALESCE(failure_code, '')::text AS failure_code,
 COALESCE(object_sha256, '')::text AS object_sha256, COALESCE(object_bytes, 0)::bigint AS object_bytes, COALESCE(row_count, 0)::bigint AS row_count, media_type,
 requested_at, as_of, ready_at, expires_at, failed_at, expired_at, COALESCE(expires_at > clock_timestamp(), false)::bool AS live
FROM org_export_jobs
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND requester_membership_id= CAST(sqlc.arg(membership_id)::text AS UUID);

-- name: ListExportsForRequester :many
SELECT CAST(id AS text) AS id, kind, parameters, parameters_digest, state, attempts, max_attempts, COALESCE(failure_code, '')::text AS failure_code,
 COALESCE(object_sha256, '')::text AS object_sha256, COALESCE(object_bytes, 0)::bigint AS object_bytes, COALESCE(row_count, 0)::bigint AS row_count, media_type,
 requested_at, as_of, ready_at, expires_at, failed_at, expired_at
FROM org_export_jobs
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND requester_membership_id= CAST(sqlc.arg(membership_id)::text AS UUID)
 AND (sqlc.arg(state)::text='' OR state= sqlc.arg(state)::text)
 AND (NOT sqlc.arg(has_after)::bool OR (requested_at, id) < (sqlc.arg(after_at)::timestamptz, CAST(sqlc.arg(after_id)::text AS UUID)))
ORDER BY requested_at DESC, id DESC LIMIT sqlc.arg(row_limit)::int;

-- name: ClaimExport :one
-- One holder at a time: a concurrent claimer skips the locked row.
UPDATE org_export_jobs SET state='RUNNING', attempts=attempts+1, lease_token= CAST(sqlc.arg(lease)::text AS UUID),
 lease_expires_at= clock_timestamp() + make_interval(secs => sqlc.arg(lease_seconds)::int)
WHERE (org_id, id)=(
 SELECT org_id, id FROM org_export_jobs
 WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND attempts<max_attempts
  AND (state='QUEUED' OR (state='RUNNING' AND lease_expires_at<=clock_timestamp()))
 FOR UPDATE SKIP LOCKED)
RETURNING CAST(requester_account_id AS text) AS requester_account_id, CAST(requester_membership_id AS text) AS requester_membership_id, kind, parameters, as_of,
 row_limit, byte_limit, time_limit_ms, retention_seconds, attempts, max_attempts, media_type,
 COALESCE(object_sha256, '')::text AS object_sha256, COALESCE(object_bytes, 0)::bigint AS object_bytes, COALESCE(row_count, 0)::bigint AS row_count;

-- name: ExportWorkState :one
SELECT state, attempts, max_attempts, COALESCE(lease_expires_at > clock_timestamp(), false)::bool AS live,
 COALESCE(object_sha256, '')::text AS object_sha256
FROM org_export_jobs WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID);

-- name: ExhaustAbandonedExport :one
-- A RUNNING job whose lease expired on its last attempt can never be claimed.
UPDATE org_export_jobs SET state='FAILED', failure_code='EXPORT_ATTEMPTS_EXHAUSTED', lease_token=NULL, lease_expires_at=NULL
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='RUNNING'
 AND lease_expires_at<=clock_timestamp() AND attempts>=max_attempts
RETURNING (object_sha256 IS NOT NULL)::bool AS has_object;

-- name: RecordProvisionalExport :execrows
UPDATE org_export_jobs SET object_sha256= sqlc.arg(object_sha256)::text, object_bytes= sqlc.arg(object_bytes)::bigint, row_count= sqlc.arg(row_count)::bigint
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='RUNNING'
 AND lease_token= CAST(sqlc.arg(lease)::text AS UUID) AND lease_expires_at>clock_timestamp() AND media_type= sqlc.arg(media_type)::text;

-- name: PublishExport :one
UPDATE org_export_jobs SET state='READY', lease_token=NULL, lease_expires_at=NULL
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='RUNNING'
 AND lease_token= CAST(sqlc.arg(lease)::text AS UUID) AND lease_expires_at>clock_timestamp()
 AND object_sha256= sqlc.arg(object_sha256)::text AND object_bytes= sqlc.arg(object_bytes)::bigint AND row_count= sqlc.arg(row_count)::bigint AND media_type= sqlc.arg(media_type)::text
RETURNING expires_at;

-- name: ReleaseExport :execrows
UPDATE org_export_jobs SET state='QUEUED', lease_token=NULL, lease_expires_at=NULL
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='RUNNING'
 AND lease_token= CAST(sqlc.arg(lease)::text AS UUID) AND attempts<max_attempts;

-- name: FailExport :one
UPDATE org_export_jobs SET state='FAILED', failure_code= sqlc.arg(failure_code)::text, lease_token=NULL, lease_expires_at=NULL
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='RUNNING'
 AND lease_token= CAST(sqlc.arg(lease)::text AS UUID)
RETURNING (object_sha256 IS NOT NULL)::bool AS has_object;

-- name: LockExportForCleanup :one
SELECT state, COALESCE(expires_at <= clock_timestamp(), false)::bool AS due, COALESCE(object_sha256, '')::text AS object_sha256,
 COALESCE(object_bytes, 0)::bigint AS object_bytes, COALESCE(row_count, 0)::bigint AS row_count, media_type, (object_deleted_at IS NOT NULL)::bool AS deleted
FROM org_export_jobs WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) FOR UPDATE;

-- name: ExpireExport :execrows
UPDATE org_export_jobs SET state='EXPIRED'
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state='READY' AND expires_at<=clock_timestamp();

-- name: MarkExportObjectDeleted :execrows
UPDATE org_export_jobs SET object_deleted_at=clock_timestamp()
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND state IN('FAILED','EXPIRED')
 AND object_sha256= sqlc.arg(object_sha256)::text AND object_deleted_at IS NULL;

-- name: AccountingConnectorFile :one
SELECT requester_account_id::text AS requester_account_id,requester_membership_id::text AS requester_membership_id,
       kind,media_type,COALESCE(object_sha256,'')::text AS sha256,
       COALESCE(object_bytes,0)::bigint AS bytes,COALESCE(row_count,0)::bigint AS rows
FROM org_export_jobs
WHERE org_id=sqlc.arg('org_id')::text::uuid AND id=sqlc.arg('id')::text::uuid
  AND state='READY' AND expires_at>clock_timestamp() AND media_type='text/csv'
  AND kind IN ('fin.accounting_competence.csv','fin.accounting_cash.csv')
  AND parameters::jsonb->'layout_version'='1'::jsonb
  AND jsonb_typeof(parameters::jsonb->'period_start')='string'
  AND jsonb_typeof(parameters::jsonb->'period_end')='string';
