-- Forum reactions and ranked conversation reads (B-BR-FOR-17, B-BR-FOR-18).

-- name: GetPublicCommentReactionTarget :one
-- A reaction target is a published, visible comment on a published, visible
-- post whose parent, when present, is also public.
SELECT c.id, c.post_id
FROM forum_comments c
JOIN forum_posts p ON p.id = c.post_id
LEFT JOIN forum_comments parent ON parent.id = c.parent_id
WHERE c.id = $1
  AND c.publication_state = 'PUBLISHED'
  AND c.visibility = 'VISIBLE'
  AND p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND (c.parent_id IS NULL OR (parent.publication_state = 'PUBLISHED' AND parent.visibility = 'VISIBLE'));

-- name: IsPublicPost :one
SELECT EXISTS (
    SELECT 1 FROM forum_posts
    WHERE id = $1 AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE'
);

-- name: UpsertCommentReaction :exec
INSERT INTO forum_comment_reactions (comment_id, account_id, post_id, value)
VALUES ($1, $2, $3, $4)
ON CONFLICT (comment_id, account_id) DO UPDATE
SET value = EXCLUDED.value, updated_at = now()
WHERE forum_comment_reactions.value <> EXCLUDED.value;

-- name: DeleteCommentReaction :exec
DELETE FROM forum_comment_reactions
WHERE comment_id = $1 AND account_id = $2;

-- name: GetCommentReactionCounts :one
SELECT like_count, dislike_count FROM forum_comments WHERE id = $1;

-- name: UpsertPostReaction :exec
INSERT INTO forum_post_reactions (post_id, account_id, value)
VALUES ($1, $2, $3)
ON CONFLICT (post_id, account_id) DO UPDATE
SET value = EXCLUDED.value, updated_at = now()
WHERE forum_post_reactions.value <> EXCLUDED.value;

-- name: DeletePostReaction :exec
DELETE FROM forum_post_reactions
WHERE post_id = $1 AND account_id = $2;

-- name: GetPostReactionCounts :one
SELECT like_count, dislike_count FROM forum_posts WHERE id = $1;

-- name: GetOwnPostReaction :one
SELECT value FROM forum_post_reactions
WHERE post_id = $1 AND account_id = $2;

-- name: ListOwnCommentReactionsForPost :many
-- Private owner read of one conversation, bounded by the caller.
SELECT comment_id, value
FROM forum_comment_reactions
WHERE account_id = $1 AND post_id = $2
ORDER BY comment_id
LIMIT $3;

-- name: ListOwnReactionsByAccount :many
-- Privacy export (B-BR-FOR-13): every reaction the account holds.
SELECT 'POST'::text AS target_type, post_id AS target_id, value, updated_at
FROM forum_post_reactions WHERE forum_post_reactions.account_id = $1
UNION ALL
SELECT 'COMMENT'::text AS target_type, comment_id AS target_id, value, updated_at
FROM forum_comment_reactions WHERE forum_comment_reactions.account_id = $1
ORDER BY target_type, target_id;

-- name: ListCommentsByPostRecent :many
SELECT *
FROM forum_comments
WHERE post_id = $1
  AND parent_id IS NULL
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND (sqlc.narg('cursor_sequence')::bigint IS NULL OR sequence < sqlc.narg('cursor_sequence'))
ORDER BY sequence DESC
LIMIT $2;

-- name: ListCommentsByPostLiked :many
SELECT *
FROM forum_comments
WHERE post_id = $1
  AND parent_id IS NULL
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND (
    sqlc.narg('cursor_score')::bigint IS NULL OR
    (like_count, sequence) < (sqlc.narg('cursor_score')::bigint, sqlc.narg('cursor_sequence')::bigint)
  )
ORDER BY like_count DESC, sequence DESC
LIMIT $2;

-- name: ListCommentsByPostRelevant :many
SELECT *
FROM forum_comments
WHERE post_id = $1
  AND parent_id IS NULL
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND (
    sqlc.narg('cursor_score')::bigint IS NULL OR
    ((like_count - dislike_count), sequence) < (sqlc.narg('cursor_score')::bigint, sqlc.narg('cursor_sequence')::bigint)
  )
ORDER BY (like_count - dislike_count) DESC, sequence DESC
LIMIT $2;

-- name: ListTopRepliesForParents :many
-- Best answer preview for each reply on a page: most liked, oldest on ties.
SELECT DISTINCT ON (parent_id) *
FROM forum_comments
WHERE parent_id = ANY(@parent_ids::uuid[])
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
ORDER BY parent_id, like_count DESC, sequence ASC;
