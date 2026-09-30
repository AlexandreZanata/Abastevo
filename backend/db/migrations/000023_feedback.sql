-- 000023: FREE station/fuel ratings (P14-T02, B-BR-F02). One live row
-- per account/station/product enforced by a partial unique index, so
-- concurrent writers serialize on the key and cannot inflate counts.
-- Edits bump the revision in place; deletes tombstone (history stays
-- for audit, aggregates ignore it). Account rows are never hard
-- deleted (status only), so the account reference needs no cascade;
-- stations are append-only under RESTRICT like the official module.
-- feedback_rating_stats is rebuildable from the live rows at any
-- time; writers maintain it in the same transaction. Append-only;
-- comments/votes arrive in later slices without rewriting this file.
CREATE TABLE feedback_ratings (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    product TEXT NOT NULL CONSTRAINT feedback_ratings_product_check CHECK (product <> ''),
    stars SMALLINT NOT NULL CONSTRAINT feedback_ratings_stars_check CHECK (stars BETWEEN 1 AND 5),
    revision INT NOT NULL DEFAULT 1 CONSTRAINT feedback_ratings_revision_check CHECK (revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX feedback_ratings_current_unique
    ON feedback_ratings (account_id, station_id, product)
    WHERE deleted_at IS NULL;
CREATE INDEX feedback_ratings_target_idx
    ON feedback_ratings (station_id, product)
    WHERE deleted_at IS NULL;
CREATE TABLE feedback_rating_stats (
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    product TEXT NOT NULL,
    ratings_count BIGINT NOT NULL DEFAULT 0,
    stars_sum BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT feedback_rating_stats_pk PRIMARY KEY (station_id, product)
);
