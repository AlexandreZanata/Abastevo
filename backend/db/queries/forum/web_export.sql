-- Owned by forum. Editorial web projection read by the enterprise export
-- (B-BR-FOR-19 / B-BR-SEO-08): published, visible EDITORIAL posts of an
-- approved, publication-enabled Abastevo editorial source. Public projection
-- fields only; author/account columns are never selected.

-- name: ExportEditorialPosts :many
SELECT
    p.id, p.category, p.title, p.excerpt, p.body_text,
    p.first_published_at, p.local_updated_at,
    p.region_uf, p.region_municipality_ibge,
    p.fuel_product, p.station_id,
    s.name AS source_name
FROM forum_posts p
JOIN forum_sources s ON s.id = p.source_id
WHERE p.kind = 'EDITORIAL'
  AND p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND s.kind = 'ABASTEVO'
  AND s.rights_status = 'APPROVED'
  AND s.publication_enabled = true
  AND (@after_id::text = '' OR p.id::text > @after_id::text)
ORDER BY p.id::text ASC
LIMIT @limit_plus_one::int;

-- name: ExportEditorialFingerprint :one
-- Cheap change detector for the export state: it covers every exported field
-- of the same predicate without returning rows. Instants enter as epoch text
-- so the session time zone cannot change the hash.
SELECT count(*)::bigint AS posts,
    COALESCE(md5(string_agg(md5(
        p.id::text || ':' || p.category || ':' || p.title || ':' || p.excerpt || ':' ||
        p.body_text || ':' || extract(epoch FROM p.first_published_at)::text || ':' ||
        extract(epoch FROM p.local_updated_at)::text || ':' ||
        COALESCE(p.region_uf, '') || ':' || COALESCE(p.region_municipality_ibge::text, '') || ':' ||
        COALESCE(p.fuel_product, '') || ':' || COALESCE(p.station_id::text, '') || ':' || s.name
    ), ',' ORDER BY p.id::text)), '')::text AS posts_hash
FROM forum_posts p
JOIN forum_sources s ON s.id = p.source_id
WHERE p.kind = 'EDITORIAL'
  AND p.publication_state = 'PUBLISHED'
  AND p.visibility = 'VISIBLE'
  AND s.kind = 'ABASTEVO'
  AND s.rights_status = 'APPROVED'
  AND s.publication_enabled = true;
