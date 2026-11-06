-- Owned by forum. Editorial post lifecycle and stable feed queries (FOR-03).

-- name: GetFeedRevision :one
SELECT revision, updated_at
FROM forum_feed_state
WHERE id = 1;

-- name: BumpFeedRevision :one
UPDATE forum_feed_state
SET revision = revision + 1, updated_at = @now
WHERE id = 1
RETURNING revision;

-- name: GetPublicPostByID :one
SELECT
    p.id, p.kind, p.category, p.title, p.excerpt, p.body_text,
    p.source_id, p.canonical_url, p.canonical_url_hash, p.external_id,
    p.author_account_id, p.public_alias,
    p.source_published_at, p.source_updated_at,
    p.local_created_at, p.local_updated_at,
    p.first_published_at, p.first_publication_seq,
    p.publication_state, p.visibility, p.revision,
    p.comments_enabled, p.correction,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    p.rights_snapshot, p.bulletin_key, p.supersedes_post_id,
    s.name AS source_name,
    s.origin_url AS source_origin_url,
    s.attribution AS source_attribution
FROM forum_posts p
LEFT JOIN forum_sources s ON s.id = p.source_id
LEFT JOIN accounts a ON a.id = p.author_account_id
WHERE p.id = @id
  AND p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND (
    (p.kind <> 'TOPIC' AND s.id IS NOT NULL AND s.rights_status = 'APPROVED' AND s.publication_enabled = true)
    OR
    (p.kind = 'TOPIC' AND a.status = 'active')
  );

-- name: GetPostByID :one
SELECT
    p.id, p.kind, p.category, p.title, p.excerpt, p.body_text,
    p.source_id, p.canonical_url, p.canonical_url_hash, p.external_id,
    p.author_account_id, p.public_alias,
    p.source_published_at, p.source_updated_at,
    p.local_created_at, p.local_updated_at,
    p.first_published_at, p.first_publication_seq,
    p.publication_state, p.visibility, p.revision,
    p.comments_enabled, p.correction,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    p.rights_snapshot, p.bulletin_key, p.supersedes_post_id,
    s.name AS source_name,
    s.origin_url AS source_origin_url,
    s.attribution AS source_attribution
FROM forum_posts p
LEFT JOIN forum_sources s ON s.id = p.source_id
WHERE p.id = @id;

-- name: GetPostByIDForUpdate :one
SELECT
    p.id, p.kind, p.category, p.title, p.excerpt, p.body_text,
    p.source_id, p.canonical_url, p.canonical_url_hash, p.external_id,
    p.author_account_id, p.public_alias,
    p.source_published_at, p.source_updated_at,
    p.local_created_at, p.local_updated_at,
    p.first_published_at, p.first_publication_seq,
    p.publication_state, p.visibility, p.revision,
    p.comments_enabled, p.correction,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    p.rights_snapshot, p.bulletin_key, p.supersedes_post_id
FROM forum_posts p
WHERE p.id = @id
FOR UPDATE;

-- name: ListPublicPosts :many
SELECT
    p.id, p.kind, p.category, p.title, p.excerpt,
    p.source_id, p.canonical_url,
    p.author_account_id, p.public_alias,
    p.source_published_at,
    p.local_updated_at,
    p.first_published_at, p.first_publication_seq,
    p.revision,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    s.name AS source_name,
    s.origin_url AS source_origin_url,
    s.attribution AS source_attribution
FROM forum_posts p
LEFT JOIN forum_sources s ON s.id = p.source_id
LEFT JOIN accounts a ON a.id = p.author_account_id
WHERE p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND (
    (p.kind <> 'TOPIC' AND s.id IS NOT NULL AND s.rights_status = 'APPROVED' AND s.publication_enabled = true)
    OR
    (p.kind = 'TOPIC' AND a.status = 'active')
  )
  AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category')::text)
  AND (sqlc.narg('kind')::text IS NULL OR p.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('uf')::text IS NULL OR p.region_uf = sqlc.narg('uf')::text)
  AND (sqlc.narg('municipality_ibge')::int IS NULL OR p.region_municipality_ibge = sqlc.narg('municipality_ibge')::int)
  AND (sqlc.narg('fuel_product')::text IS NULL OR p.fuel_product = sqlc.narg('fuel_product')::text)
  AND (sqlc.narg('station_id')::uuid IS NULL OR p.station_id = sqlc.narg('station_id')::uuid)
  AND (
    sqlc.narg('cursor_seq')::bigint IS NULL OR
    (p.first_publication_seq, p.id) < (sqlc.narg('cursor_seq')::bigint, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY p.first_publication_seq DESC, p.id DESC
LIMIT @page_limit;

-- name: AdminListPosts :many
SELECT
    p.id, p.kind, p.category, p.title, p.excerpt,
    p.source_id, p.canonical_url,
    p.author_account_id, p.public_alias,
    p.source_published_at,
    p.local_created_at, p.local_updated_at,
    p.first_published_at, p.first_publication_seq,
    p.publication_state, p.visibility, p.revision,
    p.comments_enabled, p.correction,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    s.name AS source_name,
    s.origin_url AS source_origin_url,
    s.attribution AS source_attribution
FROM forum_posts p
LEFT JOIN forum_sources s ON s.id = p.source_id
WHERE (sqlc.narg('publication_state')::text IS NULL OR p.publication_state = sqlc.narg('publication_state')::text)
  AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category')::text)
  AND (sqlc.narg('kind')::text IS NULL OR p.kind = sqlc.narg('kind')::text)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL OR
    (p.local_created_at, p.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY p.local_created_at DESC, p.id DESC
LIMIT @page_limit;

-- name: InsertEditorialPost :exec
INSERT INTO forum_posts (
    id, kind, category, title, excerpt, body_text,
    source_id, canonical_url, canonical_url_hash, external_id,
    author_account_id, public_alias,
    source_published_at, source_updated_at,
    local_created_at, local_updated_at,
    first_published_at, first_publication_seq,
    publication_state, visibility, revision,
    comments_enabled, correction,
    region_uf, region_municipality_ibge,
    fuel_product, station_id,
    author_operation_id, body_sha256, rights_snapshot, bulletin_key, supersedes_post_id
) VALUES (
    @id, @kind, @category, @title, @excerpt, @body_text,
    @source_id, @canonical_url, @canonical_url_hash, @external_id,
    @author_account_id, @public_alias,
    @source_published_at, @source_updated_at,
    @local_created_at, @local_updated_at,
    @first_published_at, @first_publication_seq,
    @publication_state, @visibility, @revision,
    @comments_enabled, @correction,
    @region_uf, @region_municipality_ibge,
    @fuel_product, @station_id,
    @author_operation_id, @body_sha256, @rights_snapshot, @bulletin_key, @supersedes_post_id
);

-- name: PublishEditorialPost :execrows
UPDATE forum_posts
SET publication_state = 'PUBLISHED',
    first_publication_seq = @first_publication_seq,
    first_published_at = COALESCE(first_published_at, @published_at),
    local_updated_at = @published_at,
    revision = revision + 1
WHERE id = @id AND publication_state <> 'PUBLISHED';

-- name: UpdateEditorialPost :execrows
UPDATE forum_posts
SET title = @title,
    excerpt = @excerpt,
    body_text = @body_text,
    category = @category,
    fuel_product = @fuel_product,
    region_uf = @region_uf,
    region_municipality_ibge = @region_municipality_ibge,
    station_id = @station_id,
    comments_enabled = @comments_enabled,
    correction = @correction,
    local_updated_at = @updated_at,
    revision = revision + 1
WHERE id = @id AND revision = @expected_revision;

-- name: SetPostVisibility :execrows
UPDATE forum_posts
SET visibility = @visibility,
    local_updated_at = @updated_at,
    revision = revision + 1
WHERE id = @id
  AND (
      (@visibility = 'VISIBLE' AND publication_state <> 'REMOVED')
      OR @visibility <> 'VISIBLE'
  );

-- name: GetSourceByID :one
SELECT * FROM forum_sources WHERE id = @id;

-- name: GetSourceByCode :one
SELECT * FROM forum_sources WHERE code = @code;

-- name: InsertSource :exec
INSERT INTO forum_sources (
    id, code, name, kind, origin_url, fetch_url, fetch_format,
    collection_enabled, publication_enabled, rights_status,
    title_allowed, excerpt_allowed, image_allowed,
    terms_url, terms_sha256, checked_at, review_expires_at,
    policy_version, attribution, admin_audit_ref,
    created_at, updated_at
) VALUES (
    @id, @code, @name, @kind, @origin_url, @fetch_url, @fetch_format,
    @collection_enabled, @publication_enabled, @rights_status,
    @title_allowed, @excerpt_allowed, @image_allowed,
    @terms_url, @terms_sha256, @checked_at, @review_expires_at,
    @policy_version, @attribution, @admin_audit_ref,
    @created_at, @updated_at
);

-- name: RecordForumChange :exec
INSERT INTO forum_changes (
    id, target_type, target_id, revision, action,
    actor_kind, actor_account_id, admin_audit_ref,
    policy_version, reason_code, prior_state, new_state, created_at
) VALUES (
    @id, @target_type, @target_id, @revision, @action,
    @actor_kind, @actor_account_id, @admin_audit_ref,
    @policy_version, @reason_code, @prior_state, @new_state, @created_at
);

-- name: ListEnabledSources :many
SELECT * FROM forum_sources
WHERE collection_enabled = true;

-- name: GetSourceCheckpoint :one
SELECT source_id, etag, last_modified, last_run_id, last_success_at, last_status, consecutive_failures, updated_at
FROM forum_source_checkpoints
WHERE source_id = @source_id;

-- name: UpsertSourceCheckpoint :exec
INSERT INTO forum_source_checkpoints (
    source_id, etag, last_modified, last_run_id, last_success_at, last_status, consecutive_failures, updated_at
) VALUES (
    @source_id, @etag, @last_modified, @last_run_id, @last_success_at, @last_status, @consecutive_failures, @updated_at
)
ON CONFLICT (source_id) DO UPDATE
SET etag = EXCLUDED.etag,
    last_modified = EXCLUDED.last_modified,
    last_run_id = EXCLUDED.last_run_id,
    last_success_at = COALESCE(EXCLUDED.last_success_at, forum_source_checkpoints.last_success_at),
    last_status = EXCLUDED.last_status,
    consecutive_failures = EXCLUDED.consecutive_failures,
    updated_at = EXCLUDED.updated_at;

-- name: InsertSourceRun :exec
INSERT INTO forum_source_runs (
    id, source_id, status, etag, last_modified,
    items_discovered, items_ingested, error_message, started_at, completed_at
) VALUES (
    @id, @source_id, @status, @etag, @last_modified,
    @items_discovered, @items_ingested, @error_message, @started_at, @completed_at
);

-- name: HasPostByExternalID :one
SELECT EXISTS (
    SELECT 1 FROM forum_posts
    WHERE source_id = @source_id AND external_id = @external_id
) AS exists;

-- name: HasPostByUrlHash :one
SELECT EXISTS (
    SELECT 1 FROM forum_posts
    WHERE source_id = @source_id AND canonical_url_hash = @canonical_url_hash
) AS exists;

-- name: ListAllSources :many
SELECT s.id, s.code, s.name, s.kind, s.origin_url, s.fetch_url, s.fetch_format,
       s.collection_enabled, s.publication_enabled, s.rights_status,
       s.title_allowed, s.excerpt_allowed, s.image_allowed,
       s.terms_url, s.terms_sha256, s.checked_at, s.review_expires_at,
       s.policy_version, s.attribution, s.admin_audit_ref,
       s.created_at, s.updated_at,
       c.etag AS checkpoint_etag,
       c.last_modified AS checkpoint_last_modified,
       c.last_status AS checkpoint_last_status,
       c.consecutive_failures AS checkpoint_failures,
       c.last_success_at AS checkpoint_last_success_at
FROM forum_sources s
LEFT JOIN forum_source_checkpoints c ON c.source_id = s.id
ORDER BY s.name ASC;

-- name: RejectPost :execrows
UPDATE forum_posts
SET publication_state = 'REJECTED',
    local_updated_at = @updated_at,
    revision = revision + 1
WHERE id = @id AND revision = @expected_revision AND publication_state <> 'REJECTED';
