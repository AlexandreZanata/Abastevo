-- 000157: Forum Abastevo editorial foundation (FOR-03, B-BR-FOR-01..07/12/14/16, BUC-FOR-01/02).
-- Append-only. Tables: forum_sources, forum_feed_state, forum_posts, forum_changes.

CREATE TABLE forum_sources (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE CHECK (octet_length(code) BETWEEN 2 AND 64),
    name TEXT NOT NULL CHECK (octet_length(name) BETWEEN 2 AND 120),
    kind TEXT NOT NULL CHECK (kind IN ('ABASTEVO', 'GOVERNMENT', 'PUBLISHER', 'DATASET')),
    origin_url TEXT NOT NULL CHECK (origin_url ~ '^https://[a-zA-Z0-9.-]+(/.*)?$'),
    fetch_url TEXT,
    fetch_format TEXT CHECK (fetch_format IS NULL OR fetch_format IN ('RSS', 'ATOM', 'JSON', 'HTML')),
    collection_enabled BOOLEAN NOT NULL DEFAULT false,
    publication_enabled BOOLEAN NOT NULL DEFAULT false,
    rights_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (rights_status IN ('PENDING', 'APPROVED', 'DENIED')),
    title_allowed BOOLEAN NOT NULL DEFAULT false,
    excerpt_allowed BOOLEAN NOT NULL DEFAULT false,
    image_allowed BOOLEAN NOT NULL DEFAULT false,
    terms_url TEXT NOT NULL DEFAULT '',
    terms_sha256 TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ,
    review_expires_at TIMESTAMPTZ,
    policy_version INT NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    attribution TEXT NOT NULL DEFAULT '',
    admin_audit_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed one official Abastevo source per ADR-019 / FORUM_ABASTEVO_ARCHITECTURE.md
INSERT INTO forum_sources (
    id, code, name, kind, origin_url,
    publication_enabled, rights_status,
    title_allowed, excerpt_allowed, image_allowed,
    attribution, created_at, updated_at
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'abastevo-editorial',
    'Equipe Abastevo',
    'ABASTEVO',
    'https://abastevo.com.br',
    true,
    'APPROVED',
    true,
    true,
    false,
    'Equipe Editorial Abastevo',
    now(),
    now()
);

CREATE TABLE forum_feed_state (
    id INT PRIMARY KEY CHECK (id = 1),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO forum_feed_state (id, revision, updated_at) VALUES (1, 0, now());

CREATE TABLE forum_posts (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('EDITORIAL', 'NEWS', 'TOPIC', 'BULLETIN')),
    category TEXT NOT NULL CHECK (category IN (
        'FUELS', 'ELECTRIC', 'TRAFFIC_RULES', 'VEHICLES_TECH',
        'ROADS_MOBILITY', 'DRIVER_ECONOMY', 'ABASTEVO_DATA'
    )),
    title TEXT NOT NULL CHECK (octet_length(title) >= 1 AND octet_length(title) <= 500 AND char_length(trim(both from title)) BETWEEN 1 AND 120),
    excerpt TEXT NOT NULL DEFAULT '' CHECK (octet_length(excerpt) <= 1600 AND char_length(excerpt) <= 400),
    body_text TEXT NOT NULL DEFAULT '' CHECK (octet_length(body_text) <= 16000 AND char_length(body_text) <= 4000),
    source_id UUID REFERENCES forum_sources(id) ON DELETE RESTRICT,
    canonical_url TEXT NOT NULL DEFAULT '' CHECK (canonical_url = '' OR canonical_url ~ '^https://[a-zA-Z0-9.-]+(/.*)?$'),
    canonical_url_hash TEXT NOT NULL DEFAULT '',
    external_id TEXT NOT NULL DEFAULT '',
    author_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    public_alias TEXT,
    source_published_at TIMESTAMPTZ,
    source_updated_at TIMESTAMPTZ,
    local_created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    local_updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_published_at TIMESTAMPTZ,
    first_publication_seq BIGINT NOT NULL DEFAULT 0 CHECK (first_publication_seq >= 0),
    publication_state TEXT NOT NULL DEFAULT 'DRAFT' CHECK (publication_state IN ('DRAFT', 'PENDING_REVIEW', 'PUBLISHED', 'REJECTED', 'REMOVED')),
    visibility TEXT NOT NULL DEFAULT 'VISIBLE' CHECK (visibility IN ('VISIBLE', 'HIDDEN')),
    revision INT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    comments_enabled BOOLEAN NOT NULL DEFAULT true,
    correction TEXT,
    region_uf CHAR(2) CHECK (region_uf IS NULL OR region_uf ~ '^[A-Z]{2}$'),
    region_municipality_ibge INT CHECK (region_municipality_ibge IS NULL OR (region_municipality_ibge >= 1000000 AND region_municipality_ibge <= 9999999)),
    fuel_product TEXT CHECK (fuel_product IS NULL OR fuel_product IN ('GASOLINE_REGULAR', 'GASOLINE_PREMIUM', 'ETHANOL', 'DIESEL_S10', 'DIESEL_S500', 'GLP', 'GNV')),
    station_id UUID REFERENCES directory_stations(id) ON DELETE RESTRICT,
    author_operation_id UUID,
    body_sha256 TEXT NOT NULL DEFAULT '',
    rights_snapshot TEXT,
    bulletin_key TEXT,
    supersedes_post_id UUID REFERENCES forum_posts(id) ON DELETE RESTRICT,
    CONSTRAINT forum_posts_source_kind_check CHECK (
        (kind = 'TOPIC' AND source_id IS NULL) OR (kind <> 'TOPIC' AND source_id IS NOT NULL)
    ),
    CONSTRAINT forum_posts_topic_author_check CHECK (
        (kind = 'TOPIC' AND publication_state <> 'REMOVED' AND author_account_id IS NOT NULL) OR
        (kind = 'TOPIC' AND publication_state = 'REMOVED') OR
        (kind <> 'TOPIC')
    ),
    CONSTRAINT forum_posts_news_source_date_check CHECK (
        (kind = 'NEWS' AND source_published_at IS NOT NULL) OR (kind <> 'NEWS')
    ),
    CONSTRAINT forum_posts_published_fields_check CHECK (
        (publication_state = 'PUBLISHED' AND first_published_at IS NOT NULL AND first_publication_seq > 0) OR
        (publication_state <> 'PUBLISHED')
    )
);

CREATE INDEX forum_posts_feed_idx
    ON forum_posts (first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE INDEX forum_posts_feed_category_idx
    ON forum_posts (category, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE INDEX forum_posts_feed_kind_idx
    ON forum_posts (kind, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE INDEX forum_posts_feed_cat_kind_idx
    ON forum_posts (category, kind, first_publication_seq DESC, id DESC)
    WHERE publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE UNIQUE INDEX forum_posts_source_ext_uniq
    ON forum_posts (source_id, external_id)
    WHERE external_id <> '';

CREATE UNIQUE INDEX forum_posts_source_url_hash_uniq
    ON forum_posts (source_id, canonical_url_hash)
    WHERE canonical_url_hash <> '';

CREATE INDEX forum_posts_author_created_idx
    ON forum_posts (author_account_id, local_created_at, id)
    WHERE author_account_id IS NOT NULL;

CREATE UNIQUE INDEX forum_posts_author_op_uniq
    ON forum_posts (author_account_id, author_operation_id)
    WHERE author_account_id IS NOT NULL AND author_operation_id IS NOT NULL;

CREATE UNIQUE INDEX forum_posts_bulletin_key_uniq
    ON forum_posts (bulletin_key)
    WHERE bulletin_key IS NOT NULL;

CREATE TABLE forum_changes (
    id UUID PRIMARY KEY,
    target_type TEXT NOT NULL CHECK (target_type IN ('POST', 'SOURCE')),
    target_id UUID NOT NULL,
    revision INT NOT NULL CHECK (revision >= 1),
    action TEXT NOT NULL CHECK (octet_length(action) BETWEEN 2 AND 64),
    actor_kind TEXT NOT NULL CHECK (actor_kind IN ('ADMIN', 'SYSTEM', 'AUTHOR')),
    actor_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    admin_audit_ref TEXT NOT NULL DEFAULT '',
    policy_version INT NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    reason_code TEXT NOT NULL CHECK (octet_length(reason_code) BETWEEN 2 AND 64),
    prior_state TEXT NOT NULL DEFAULT '',
    new_state TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT forum_changes_uniq UNIQUE (target_type, target_id, revision, action)
);
