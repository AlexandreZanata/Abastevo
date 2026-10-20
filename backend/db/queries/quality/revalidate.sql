-- FQ-05W6 Directory revalidation reads. Only currently published revisions
-- are compared with live Directory evidence; superseded/withdrawn revisions
-- remain frozen historical facts and are never rewritten.

-- name: GetRevalidationCursor :one
SELECT kind, cursor_edition, cursor_key, updated_at
FROM quality_revalidation_cursors WHERE kind = @kind;

-- name: SetRevalidationCursor :exec
INSERT INTO quality_revalidation_cursors (kind, cursor_edition, cursor_key)
VALUES (@kind, @cursor_edition, @cursor_key)
ON CONFLICT (kind) DO UPDATE
 SET cursor_edition = EXCLUDED.cursor_edition,
     cursor_key = EXCLUDED.cursor_key,
     updated_at = now();

-- name: ListPublishedLinkEditions :many
-- Editions with at least one published linked record, ordered by stable
-- identifier so a durable edition cursor always advances. Whole editions
-- are judged at once, so a batch boundary never splits one association set.
SELECT DISTINCT e.id AS edition_id, e.source, e.stream, e.edition, r.id AS revision_id
FROM quality_revisions r
JOIN quality_editions e ON e.id = r.edition_id
JOIN quality_links l ON l.revision_id = r.id
JOIN quality_assays a ON a.revision_id = l.revision_id
 AND l.record_key = 'pmqc-assay-v1:' || a.sample_ref
WHERE r.status = 'published'
 AND (sqlc.narg('cursor_edition')::uuid IS NULL OR e.id > sqlc.narg('cursor_edition')::uuid)
ORDER BY e.id
LIMIT sqlc.arg('row_limit');

-- name: ListRevisionLinkRows :many
-- Every published link of one frozen revision with the record's source
-- identity derived from its own ledgers. One row per record key.
SELECT DISTINCT ON (l.record_key)
 l.record_key, l.station_id, l.active,
 a.source_cnpj, a.observed_on
FROM quality_links l
JOIN quality_assays a ON a.revision_id = l.revision_id
 AND l.record_key = 'pmqc-assay-v1:' || a.sample_ref
WHERE l.revision_id = @revision_id
ORDER BY l.record_key;
