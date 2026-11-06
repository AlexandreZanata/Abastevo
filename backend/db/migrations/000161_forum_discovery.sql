-- 000161: Forum Abastevo bounded subject filters (FOR-24).
-- Append-only. Adds selective region, fuel and station feed indices.
-- Portuguese FTS remains SEARCH_DEFERRED: measured 10k/100k plans prefer the
-- ordered feed index over a GIN bitmap plus sort, so no search column/index is
-- added before a measured use case justifies it.

CREATE INDEX forum_posts_feed_uf_idx
    ON forum_posts (region_uf, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE' AND region_uf IS NOT NULL;

CREATE INDEX forum_posts_feed_municipality_idx
    ON forum_posts (region_municipality_ibge, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE' AND region_municipality_ibge IS NOT NULL;

CREATE INDEX forum_posts_feed_fuel_idx
    ON forum_posts (fuel_product, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE' AND fuel_product IS NOT NULL;

CREATE INDEX forum_posts_feed_station_idx
    ON forum_posts (station_id, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE' AND station_id IS NOT NULL;

