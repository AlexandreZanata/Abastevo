-- name: InsertDocumentLink :exec
INSERT INTO org_document_links(org_id, document_id, target_kind, target_id, asset_id, employee_id, reading_id, held_reading_id, incident_report_id, fueling_id, operation_id, created_by)
VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(document_id)::text AS UUID), sqlc.arg(target_kind)::text, CAST(sqlc.arg(target_id)::text AS UUID),
 CAST(sqlc.narg(asset_id)::text AS UUID), CAST(sqlc.narg(employee_id)::text AS UUID), CAST(sqlc.narg(reading_id)::text AS UUID),
 CAST(sqlc.narg(held_reading_id)::text AS UUID), CAST(sqlc.narg(incident_report_id)::text AS UUID), CAST(sqlc.narg(fueling_id)::text AS UUID),
 CAST(sqlc.arg(operation_id)::text AS UUID), CAST(sqlc.arg(created_by)::text AS UUID));

-- name: DocumentLinkByOperation :one
SELECT CAST(l.document_id AS text) AS document_id, l.target_kind, CAST(l.target_id AS text) AS target_id, CAST(l.operation_id AS text) AS operation_id,
 CAST(l.created_by AS text) AS created_by, d.storage_state, l.created_at
FROM org_document_links l JOIN org_documents d ON d.org_id=l.org_id AND d.id=l.document_id
WHERE l.org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND l.operation_id= CAST(sqlc.arg(operation_id)::text AS UUID);

-- name: DocumentLinkByTarget :one
SELECT CAST(l.document_id AS text) AS document_id, l.target_kind, CAST(l.target_id AS text) AS target_id, CAST(l.operation_id AS text) AS operation_id,
 CAST(l.created_by AS text) AS created_by, d.storage_state, l.created_at
FROM org_document_links l JOIN org_documents d ON d.org_id=l.org_id AND d.id=l.document_id
WHERE l.org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND l.document_id= CAST(sqlc.arg(document_id)::text AS UUID)
 AND l.target_kind= sqlc.arg(target_kind)::text AND l.target_id= CAST(sqlc.arg(target_id)::text AS UUID);

-- name: DocumentLinksByDocument :many
SELECT CAST(l.document_id AS text) AS document_id, l.target_kind, CAST(l.target_id AS text) AS target_id, CAST(l.operation_id AS text) AS operation_id,
 CAST(l.created_by AS text) AS created_by, d.storage_state, l.created_at
FROM org_document_links l JOIN org_documents d ON d.org_id=l.org_id AND d.id=l.document_id
WHERE l.org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND l.document_id= CAST(sqlc.arg(document_id)::text AS UUID)
 AND (sqlc.narg(after_kind)::text IS NULL
  OR (l.target_kind COLLATE "C", l.target_id) > (sqlc.narg(after_kind)::text COLLATE "C", CAST(sqlc.narg(after_target)::text AS UUID)))
ORDER BY l.target_kind COLLATE "C", l.target_id
LIMIT sqlc.arg(row_limit)::int;

-- name: DocumentLinksByTarget :many
SELECT CAST(l.document_id AS text) AS document_id, l.target_kind, CAST(l.target_id AS text) AS target_id, CAST(l.operation_id AS text) AS operation_id,
 CAST(l.created_by AS text) AS created_by, d.storage_state, l.created_at
FROM org_document_links l JOIN org_documents d ON d.org_id=l.org_id AND d.id=l.document_id
WHERE l.org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND l.target_kind= sqlc.arg(target_kind)::text AND l.target_id= CAST(sqlc.arg(target_id)::text AS UUID)
 AND (sqlc.narg(after_document)::text IS NULL OR l.document_id > CAST(sqlc.narg(after_document)::text AS UUID))
ORDER BY l.document_id
LIMIT sqlc.arg(row_limit)::int;
