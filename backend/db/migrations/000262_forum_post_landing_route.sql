-- 000262: Forum editorial landing guide/tool route (SEO-20 / B-BR-SEO-08).
-- Additive nullable column allowing an EDITORIAL post to reference its owning
-- landing guide or calculator route (/guias/{slug}/ or /ferramentas/{slug}/)
-- so the static portal can link the post back to the pillar guide without
-- allowing arbitrary URLs or HTML in the plain-text post body.
ALTER TABLE forum_posts
    ADD COLUMN landing_route TEXT
    CONSTRAINT forum_posts_landing_route_check CHECK (
        landing_route IS NULL OR (
            octet_length(landing_route) <= 120
            AND landing_route ~ '^/(guias|ferramentas)/[a-z0-9]+(-[a-z0-9]+)*/$'
        )
    );
