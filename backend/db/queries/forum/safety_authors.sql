-- Author lookups for community safety (B-BR-SAFE-02/06). Report and block
-- name an author only while public reads serve the content; the hidden
-- filter resolves authors of any listed id. Callers never project the
-- account id to clients.

-- name: GetSafetyTopicAuthor :one
SELECT author_account_id, coalesce(public_alias, '')::text AS public_alias
FROM forum_posts
WHERE id = @id
  AND kind = 'TOPIC'
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND author_account_id IS NOT NULL;

-- name: GetSafetyCommentAuthor :one
SELECT c.author_account_id, c.public_alias
FROM forum_comments c
JOIN forum_posts p ON p.id = c.post_id
LEFT JOIN forum_comments parent ON parent.id = c.parent_id
WHERE c.id = @id
  AND c.author_account_id IS NOT NULL
  AND c.publication_state = 'PUBLISHED'
  AND c.visibility = 'VISIBLE'
  AND p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND (c.parent_id IS NULL
       OR (parent.publication_state = 'PUBLISHED' AND parent.visibility = 'VISIBLE'));

-- name: ListSafetyTopicAuthors :many
SELECT id, author_account_id
FROM forum_posts
WHERE id = ANY(@ids::uuid[])
  AND kind = 'TOPIC'
  AND author_account_id IS NOT NULL;

-- name: ListSafetyCommentAuthors :many
SELECT id, author_account_id
FROM forum_comments
WHERE id = ANY(@ids::uuid[])
  AND author_account_id IS NOT NULL;
