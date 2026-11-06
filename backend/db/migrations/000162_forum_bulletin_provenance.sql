-- 000162: Forum deterministic bulletin provenance and correction grouping
-- (FOR-27). Append-only. Existing posts keep NULL bulletin fields.

ALTER TABLE forum_posts ADD COLUMN bulletin_group_key TEXT;
ALTER TABLE forum_posts ADD COLUMN bulletin_provenance JSONB;

ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_bulletin_provenance_shape_check
    CHECK (bulletin_provenance IS NULL OR jsonb_typeof(bulletin_provenance) = 'object');

ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_bulletin_shape_check CHECK (
    (kind = 'BULLETIN'
        AND bulletin_key IS NOT NULL
        AND bulletin_group_key IS NOT NULL
        AND bulletin_provenance IS NOT NULL
        AND fuel_product IS NULL
        AND station_id IS NULL)
    OR (kind <> 'BULLETIN')
);

CREATE INDEX forum_posts_bulletin_group_idx
    ON forum_posts (bulletin_group_key, local_created_at DESC, id DESC)
    WHERE kind = 'BULLETIN' AND bulletin_group_key IS NOT NULL;
