-- name: CountDocumentStorageBytes :one
SELECT COALESCE(sum(original_bytes),0)::bigint AS bytes
FROM org_documents
WHERE org_id=sqlc.arg('org_id')::text::uuid
  AND storage_state IN ('UPLOADING','RECEIVED','STORED','QUARANTINED','REPLACED');
