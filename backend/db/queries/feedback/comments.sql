-- Owned by feedback. Bounded plain-text comments and one-level
-- replies (P14-T03, B-BR-F03/F04). Edits compare-and-swap the
-- revision; deletes tombstone. Reads resolve the opaque account
-- alias and never expose addresses or provider subjects.

-- name: LockComment :one
SELECT id, account_id, station_id, product, parent_id, depth, text,
    revision, created_at, updated_at, deleted_at, visibility
FROM feedback_comments
WHERE id = @id
FOR UPDATE;

-- name: InsertComment :exec
INSERT INTO feedback_comments
    (id, account_id, station_id, product, parent_id, depth, text,
     revision, created_at, updated_at, deleted_at)
VALUES (@id, @account_id, @station_id, @product, @parent_id, @depth,
    @text, 1, @now, @now, NULL);

-- name: UpdateCommentText :execrows
UPDATE feedback_comments
SET text = @text, revision = revision + 1, updated_at = @now
WHERE id = @id AND revision = @expected_revision AND deleted_at IS NULL;

-- name: TombstoneComment :execrows
UPDATE feedback_comments
SET deleted_at = @now
WHERE id = @id AND account_id = @account_id AND deleted_at IS NULL;

-- name: GetCommentView :one
SELECT c.id, c.account_id, a.alias, c.station_id, c.product,
    c.parent_id, c.depth, c.text, c.revision, c.created_at, c.updated_at,
    c.visibility
FROM feedback_comments c
JOIN accounts a ON a.id = c.account_id
WHERE c.id = @id AND c.deleted_at IS NULL AND c.visibility <> 'hidden';

-- name: SetVisibility :exec
UPDATE feedback_comments SET visibility = @visibility WHERE id = @id;

-- name: FlagComment :execrows
UPDATE feedback_comments SET visibility = 'flagged'
WHERE id = @id AND visibility = 'visible' AND deleted_at IS NULL;

-- name: ListTopLevel :many
SELECT c.id, c.account_id, a.alias, c.station_id, c.product,
    c.parent_id, c.depth, c.text, c.revision, c.created_at, c.updated_at,
    c.visibility
FROM feedback_comments c
JOIN accounts a ON a.id = c.account_id
WHERE c.station_id = @station_id AND c.product = @product
    AND c.parent_id IS NULL AND c.deleted_at IS NULL AND c.visibility <> 'hidden'
    AND (c.created_at, c.id) > (@after_at, @after_id::uuid)
ORDER BY c.created_at, c.id
LIMIT @page_limit;

-- name: ListReplies :many
SELECT c.id, c.account_id, a.alias, c.station_id, c.product,
    c.parent_id, c.depth, c.text, c.revision, c.created_at, c.updated_at,
    c.visibility
FROM feedback_comments c
JOIN accounts a ON a.id = c.account_id
WHERE c.parent_id = @parent_id AND c.deleted_at IS NULL AND c.visibility <> 'hidden'
    AND (c.created_at, c.id) > (@after_at, @after_id::uuid)
ORDER BY c.created_at, c.id
LIMIT @page_limit;
