-- Author lookups for community safety (B-BR-SAFE-02/06). Report and block
-- name an author only while public reads serve the comment; the hidden
-- filter resolves authors of any listed id. Callers never project the
-- account id to clients.

-- name: GetSafetyCommentAuthor :one
SELECT c.account_id, COALESCE(a.display_name, a.alias)::text AS alias
FROM feedback_comments c
JOIN accounts a ON a.id = c.account_id
WHERE c.id = @id AND c.deleted_at IS NULL AND c.visibility <> 'hidden';

-- name: ListSafetyCommentAuthors :many
SELECT id, account_id
FROM feedback_comments
WHERE id = ANY(@ids::uuid[]);
