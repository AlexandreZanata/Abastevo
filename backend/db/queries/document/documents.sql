-- name: InsertDocument :exec
INSERT INTO org_documents(org_id, id, kind, data_mode, operation_id, created_by, original_sha256, original_bytes, original_type,
 storage_state, storage_revision, extraction_state, extraction_revision, structure_state, structure_revision,
 conference_state, conference_revision, reconciliation_state, fiscal_state, fiscal_revision)
VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(id)::text AS UUID), sqlc.arg(kind)::text, 'SYNTHETIC',
 CAST(sqlc.arg(operation_id)::text AS UUID), CAST(sqlc.arg(created_by)::text AS UUID), sqlc.arg(original_sha256)::text,
 sqlc.arg(original_bytes)::bigint, sqlc.arg(original_type)::text,
 'UPLOADING', 1, 'NONE', 1, 'NOT_APPLICABLE', 1, 'UNCONFERRED', 1, 'UNALLOCATED', 'NOT_CONSULTED', 1);

-- name: DocumentForShare :one
SELECT CAST(id AS text) AS id, kind, data_mode, CAST(operation_id AS text) AS operation_id, CAST(created_by AS text) AS created_by,
 original_sha256, original_bytes, original_type, storage_state, storage_revision, extraction_state, extraction_revision,
 structure_state, structure_revision, conference_state, conference_revision, reconciliation_state, fiscal_state,
 fiscal_revision, fiscal_consulted_at, created_at, updated_at
FROM org_documents WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) FOR SHARE;

-- name: DocumentForUpdate :one
SELECT CAST(id AS text) AS id, kind, data_mode, CAST(operation_id AS text) AS operation_id, CAST(created_by AS text) AS created_by,
 original_sha256, original_bytes, original_type, storage_state, storage_revision, extraction_state, extraction_revision,
 structure_state, structure_revision, conference_state, conference_revision, reconciliation_state, fiscal_state,
 fiscal_revision, fiscal_consulted_at, created_at, updated_at
FROM org_documents WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) FOR UPDATE;

-- name: DocumentCreator :one
SELECT CAST(created_by AS text) AS created_by FROM org_documents
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID);

-- name: AdvanceStorage :execrows
UPDATE org_documents SET storage_state= sqlc.arg(to_state)::text, storage_revision=storage_revision+1
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID)
 AND storage_state= sqlc.arg(from_state)::text AND storage_revision= sqlc.arg(expected_revision)::bigint;
