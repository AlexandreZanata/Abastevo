-- name: ReadKeyMetadata :one
SELECT id::text AS id,label,scopes,state,revision,issued_at,expires_at,overlap_until
FROM org_data_api_keys
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND id=CAST(sqlc.arg('id')::text AS uuid);

-- name: ListKeyMetadata :many
SELECT id::text AS id,label,scopes,state,revision,issued_at,expires_at,overlap_until
FROM org_data_api_keys
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid)
  AND (sqlc.arg('after_id')::text='' OR id>CAST(NULLIF(sqlc.arg('after_id')::text,'') AS uuid))
ORDER BY id LIMIT sqlc.arg('row_limit')::integer;

-- name: LockKeyProof :one
SELECT id::text AS id,label,scopes,state,revision,issued_at,expires_at,overlap_until,verifier
FROM org_data_api_keys
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND id=CAST(sqlc.arg('id')::text AS uuid)
FOR SHARE;

-- name: InsertKey :exec
INSERT INTO org_data_api_keys (org_id,id,label,scopes,verifier,state,issued_at,expires_at)
VALUES(CAST(sqlc.arg('org_id')::text AS uuid),CAST(sqlc.arg('id')::text AS uuid),
       sqlc.arg('label')::text,sqlc.arg('scopes')::text[],sqlc.arg('verifier')::text,'ACTIVE',sqlc.arg('issued_at')::timestamptz,sqlc.arg('expires_at')::timestamptz);

-- name: RotateKey :execrows
UPDATE org_data_api_keys SET state='ROTATING',revision=revision+1,overlap_until=sqlc.arg('overlap_until')::timestamptz,
    replacement_id=CAST(sqlc.arg('replacement_id')::text AS uuid)
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND id=CAST(sqlc.arg('id')::text AS uuid)
  AND state='ACTIVE' AND revision=sqlc.arg('expected_revision')::bigint;

-- name: RevokeKey :execrows
UPDATE org_data_api_keys SET state='REVOKED',revision=revision+1,revoked_at=sqlc.arg('revoked_at')::timestamptz
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND id=CAST(sqlc.arg('id')::text AS uuid)
  AND state<>'REVOKED' AND revision=sqlc.arg('expected_revision')::bigint;

-- name: KeyClock :one
SELECT pg_catalog.clock_timestamp()::timestamptz AS observed_at;

-- name: LockDataOrganization :one
SELECT state::text FROM organizations
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND has_snapshot
FOR SHARE;
