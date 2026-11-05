-- name: LockPostForComment :one
SELECT id, comments_enabled, publication_state, visibility, next_comment_sequence
FROM forum_posts
WHERE id = $1
FOR UPDATE;

-- name: BumpPostCommentSequence :exec
UPDATE forum_posts
SET next_comment_sequence = next_comment_sequence + 1,
    local_updated_at = now()
WHERE id = $1;

-- name: LockParentComment :one
SELECT id, post_id, depth, publication_state, visibility
FROM forum_comments
WHERE id = $1
FOR UPDATE;

-- name: InsertComment :one
INSERT INTO forum_comments (
    id,
    post_id,
    author_account_id,
    public_alias,
    parent_id,
    depth,
    sequence,
    body_text,
    revision,
    publication_state,
    visibility,
    created_at,
    updated_at,
    published_at,
    create_operation_id,
    body_sha256
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
RETURNING *;

-- name: GetCommentByID :one
SELECT *
FROM forum_comments
WHERE id = $1;

-- name: GetCommentByIDForUpdate :one
SELECT *
FROM forum_comments
WHERE id = $1
FOR UPDATE;

-- name: GetCommentByOperationID :one
SELECT *
FROM forum_comments
WHERE author_account_id = $1 AND create_operation_id = $2;

-- name: CountLiveCommentsByAuthor :one
SELECT count(*)
FROM forum_comments
WHERE author_account_id = $1
  AND publication_state IN ('PENDING_REVIEW', 'PUBLISHED')
  AND visibility = 'VISIBLE';

-- name: ListCommentsByPost :many
SELECT *
FROM forum_comments
WHERE post_id = $1
  AND parent_id IS NULL
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND (sqlc.narg('cursor_sequence')::bigint IS NULL OR sequence > sqlc.narg('cursor_sequence'))
ORDER BY sequence ASC, id ASC
LIMIT $2;

-- name: ListRepliesByParent :many
SELECT *
FROM forum_comments
WHERE parent_id = $1
  AND publication_state = 'PUBLISHED'
  AND visibility = 'VISIBLE'
  AND (sqlc.narg('cursor_sequence')::bigint IS NULL OR sequence > sqlc.narg('cursor_sequence'))
ORDER BY sequence ASC, id ASC
LIMIT $2;

-- name: UpdateCommentCAS :one
UPDATE forum_comments
SET body_text = $4,
    body_sha256 = $5,
    revision = revision + 1,
    publication_state = 'PENDING_REVIEW',
    updated_at = now()
WHERE id = $1
  AND author_account_id = $2
  AND revision = $3
  AND publication_state <> 'REMOVED'
RETURNING *;

-- name: TombstoneComment :one
UPDATE forum_comments
SET body_text = '',
    body_sha256 = '',
    publication_state = 'REMOVED',
    visibility = 'HIDDEN',
    revision = revision + 1,
    updated_at = now()
WHERE id = $1
  AND author_account_id = $2
  AND revision = $3
  AND publication_state <> 'REMOVED'
RETURNING *;

-- name: PublishComment :one
UPDATE forum_comments
SET publication_state = 'PUBLISHED',
    published_at = now(),
    updated_at = now(),
    revision = revision + 1
WHERE id = $1
  AND publication_state = 'PENDING_REVIEW'
RETURNING *;

-- name: RejectComment :one
UPDATE forum_comments
SET publication_state = 'REMOVED',
    visibility = 'HIDDEN',
    updated_at = now(),
    revision = revision + 1
WHERE id = $1
  AND publication_state = 'PENDING_REVIEW'
RETURNING *;

-- name: ListCommentsByAuthor :many
SELECT *
FROM forum_comments
WHERE author_account_id = $1
ORDER BY id ASC;

-- name: PendingEraseCommentsByAuthor :many
SELECT id
FROM forum_comments
WHERE author_account_id = $1
  AND (publication_state <> 'REMOVED' OR body_text <> '')
ORDER BY id ASC
LIMIT $2;

-- name: ScrubCommentForErasure :one
UPDATE forum_comments
SET body_text = '',
    body_sha256 = '',
    publication_state = 'REMOVED',
    visibility = 'HIDDEN',
    revision = revision + 1,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: HasPendingAuthorCommentErasure :one
SELECT EXISTS (
    SELECT 1
    FROM forum_comments
    WHERE author_account_id = $1
      AND (publication_state <> 'REMOVED' OR body_text <> '')
);
