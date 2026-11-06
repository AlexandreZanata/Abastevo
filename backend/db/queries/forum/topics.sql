-- Owned by forum. User topics lifecycle, CAS edit, tombstone removal, and quota queries (FOR-20).

-- name: GetTopicByOperationID :one
SELECT * FROM forum_posts
WHERE author_account_id = @author_account_id
  AND author_operation_id = @author_operation_id
  AND kind = 'TOPIC';

-- name: CountLiveTopicsByAuthor :one
SELECT COUNT(*) FROM forum_posts
WHERE author_account_id = @author_account_id
  AND kind = 'TOPIC'
  AND publication_state <> 'REMOVED';

-- name: InsertTopic :one
INSERT INTO forum_posts (
    id, kind, category, title, excerpt, body_text,
    author_account_id, public_alias,
    local_created_at, local_updated_at,
    publication_state, visibility, revision,
    comments_enabled, region_uf, region_municipality_ibge,
    fuel_product, station_id,
    author_operation_id, body_sha256
) VALUES (
    @id, 'TOPIC', @category, @title, @excerpt, @body_text,
    @author_account_id, @public_alias,
    @local_created_at, @local_updated_at,
    'PENDING_REVIEW', 'VISIBLE', 1,
    @comments_enabled, @region_uf, @region_municipality_ibge,
    @fuel_product, @station_id,
    @author_operation_id, @body_sha256
)
RETURNING *;

-- name: GetTopicByIDForUpdate :one
SELECT * FROM forum_posts
WHERE id = @id AND kind = 'TOPIC'
FOR UPDATE;

-- name: UpdateTopicCAS :one
UPDATE forum_posts
SET title = @title,
    excerpt = @excerpt,
    body_text = @body_text,
    category = @category,
    region_uf = @region_uf,
    region_municipality_ibge = @region_municipality_ibge,
    fuel_product = @fuel_product,
    station_id = @station_id,
    body_sha256 = @body_sha256,
    local_updated_at = @local_updated_at,
    revision = revision + 1,
    publication_state = 'PENDING_REVIEW'
WHERE id = @id
  AND author_account_id = @author_account_id
  AND revision = @expected_revision
  AND publication_state <> 'REMOVED'
RETURNING *;

-- name: RemoveTopicTombstone :one
UPDATE forum_posts
SET title = '[Tópico removido pelo autor]',
    excerpt = '',
    body_text = '',
    public_alias = NULL,
    publication_state = 'REMOVED',
    visibility = 'HIDDEN',
    local_updated_at = @local_updated_at,
    revision = revision + 1
WHERE id = @id
  AND author_account_id = @author_account_id
  AND revision = @expected_revision
  AND publication_state <> 'REMOVED'
RETURNING *;

-- name: GetTopicByID :one
SELECT * FROM forum_posts
WHERE id = @id AND kind = 'TOPIC';

-- name: ListTopicsByAuthor :many
SELECT *
FROM forum_posts
WHERE author_account_id = $1
  AND kind = 'TOPIC'
ORDER BY id ASC;

-- name: PendingEraseTopicsByAuthor :many
SELECT id
FROM forum_posts
WHERE author_account_id = $1
  AND kind = 'TOPIC'
  AND (publication_state <> 'REMOVED' OR body_text <> '' OR public_alias IS NOT NULL)
ORDER BY id ASC
LIMIT $2;

-- name: ScrubTopicForErasure :one
UPDATE forum_posts
SET title = '[Tópico removido]',
    excerpt = '',
    body_text = '',
    body_sha256 = '',
    public_alias = NULL,
    publication_state = 'REMOVED',
    visibility = 'HIDDEN',
    revision = revision + 1,
    local_updated_at = now()
WHERE id = $1
RETURNING *;

-- name: HasPendingAuthorTopicErasure :one
SELECT EXISTS (
    SELECT 1
    FROM forum_posts
    WHERE author_account_id = $1
      AND kind = 'TOPIC'
      AND (publication_state <> 'REMOVED' OR body_text <> '' OR public_alias IS NOT NULL)
);
