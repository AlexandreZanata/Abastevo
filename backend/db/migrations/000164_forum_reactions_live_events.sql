-- 000164: Forum reactions and live conversation events (B-BR-FOR-17,
-- B-BR-FOR-18). Append-only.
--
-- One reaction per account and post/comment (LIKE = 1, DISLIKE = -1; NONE
-- deletes the row). Triggers keep the denormalized counts exact on every
-- insert, switch and delete, including account erasure, so no code path can
-- drift them. Committed public changes are announced on the `forum_events`
-- channel; pg_notify inside the trigger is delivered only on commit.

ALTER TABLE forum_posts
    ADD COLUMN like_count BIGINT NOT NULL DEFAULT 0 CHECK (like_count >= 0),
    ADD COLUMN dislike_count BIGINT NOT NULL DEFAULT 0 CHECK (dislike_count >= 0);

ALTER TABLE forum_comments
    ADD COLUMN like_count BIGINT NOT NULL DEFAULT 0 CHECK (like_count >= 0),
    ADD COLUMN dislike_count BIGINT NOT NULL DEFAULT 0 CHECK (dislike_count >= 0);

CREATE TABLE forum_post_reactions (
    post_id UUID NOT NULL REFERENCES forum_posts(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    value SMALLINT NOT NULL CHECK (value IN (-1, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, account_id)
);

CREATE INDEX forum_post_reactions_account_idx
    ON forum_post_reactions (account_id);

CREATE TABLE forum_comment_reactions (
    comment_id UUID NOT NULL REFERENCES forum_comments(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    -- Denormalized so an owner read of one conversation is a single range scan.
    post_id UUID NOT NULL REFERENCES forum_posts(id) ON DELETE CASCADE,
    value SMALLINT NOT NULL CHECK (value IN (-1, 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (comment_id, account_id)
);

CREATE INDEX forum_comment_reactions_account_post_idx
    ON forum_comment_reactions (account_id, post_id);

-- Ranked top-level replies (B-BR-FOR-18): keyset scans for `relevant`
-- (likes minus dislikes) and `liked`, newest first on ties.
CREATE INDEX forum_comments_post_net_idx
    ON forum_comments (post_id, (like_count - dislike_count) DESC, sequence DESC)
    WHERE parent_id IS NULL AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE INDEX forum_comments_post_liked_idx
    ON forum_comments (post_id, like_count DESC, sequence DESC)
    WHERE parent_id IS NULL AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

-- Best answer preview per reply: most liked first, oldest on ties.
CREATE INDEX forum_comments_reply_liked_idx
    ON forum_comments (parent_id, like_count DESC, sequence ASC)
    WHERE parent_id IS NOT NULL AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

CREATE FUNCTION forum_comment_reaction_counts() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_value SMALLINT := 0;
    new_value SMALLINT := 0;
    target UUID;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        old_value := OLD.value;
        target := OLD.comment_id;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        new_value := NEW.value;
        target := NEW.comment_id;
    END IF;
    IF old_value = new_value THEN
        RETURN NULL;
    END IF;
    UPDATE forum_comments
    SET like_count = like_count + (new_value = 1)::int - (old_value = 1)::int,
        dislike_count = dislike_count + (new_value = -1)::int - (old_value = -1)::int
    WHERE id = target;
    RETURN NULL;
END $$;

CREATE TRIGGER forum_comment_reaction_counts
AFTER INSERT OR UPDATE OF value OR DELETE ON forum_comment_reactions
FOR EACH ROW EXECUTE FUNCTION forum_comment_reaction_counts();

CREATE FUNCTION forum_post_reaction_counts() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_value SMALLINT := 0;
    new_value SMALLINT := 0;
    target UUID;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        old_value := OLD.value;
        target := OLD.post_id;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        new_value := NEW.value;
        target := NEW.post_id;
    END IF;
    IF old_value = new_value THEN
        RETURN NULL;
    END IF;
    UPDATE forum_posts
    SET like_count = like_count + (new_value = 1)::int - (old_value = 1)::int,
        dislike_count = dislike_count + (new_value = -1)::int - (old_value = -1)::int
    WHERE id = target;
    RETURN NULL;
END $$;

CREATE TRIGGER forum_post_reaction_counts
AFTER INSERT OR UPDATE OF value OR DELETE ON forum_post_reactions
FOR EACH ROW EXECUTE FUNCTION forum_post_reaction_counts();

-- Account erasure removes the account's reactions; the count triggers above
-- take their effect off the totals in the same transaction.
CREATE FUNCTION forum_erase_deleted_owner_reactions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM forum_comment_reactions WHERE account_id = NEW.id;
    DELETE FROM forum_post_reactions WHERE account_id = NEW.id;
    RETURN NEW;
END $$;

CREATE TRIGGER forum_erase_deleted_owner_reactions AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION forum_erase_deleted_owner_reactions();

-- Live events carry identifiers and counts only; the API resolves public
-- projections before anything reaches a client.
CREATE FUNCTION forum_comment_events() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    was_public BOOLEAN := false;
    is_public BOOLEAN := NEW.publication_state = 'PUBLISHED' AND NEW.visibility = 'VISIBLE';
    kind TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF is_public THEN
            kind := 'comment';
        END IF;
    ELSE
        was_public := OLD.publication_state = 'PUBLISHED' AND OLD.visibility = 'VISIBLE';
        IF is_public AND NOT was_public THEN
            kind := 'comment';
        ELSIF was_public AND NOT is_public THEN
            kind := 'comment_gone';
        ELSIF is_public AND (NEW.like_count <> OLD.like_count OR NEW.dislike_count <> OLD.dislike_count) THEN
            kind := 'comment_reactions';
        END IF;
    END IF;
    IF kind IS NOT NULL THEN
        PERFORM pg_notify('forum_events', json_build_object(
            'type', kind,
            'post', NEW.post_id,
            'comment', NEW.id,
            'parent', NEW.parent_id,
            'likes', NEW.like_count,
            'dislikes', NEW.dislike_count
        )::text);
    END IF;
    RETURN NULL;
END $$;

CREATE TRIGGER forum_comment_events
AFTER INSERT OR UPDATE ON forum_comments
FOR EACH ROW EXECUTE FUNCTION forum_comment_events();

CREATE FUNCTION forum_post_events() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    was_public BOOLEAN := OLD.publication_state = 'PUBLISHED' AND OLD.visibility = 'VISIBLE';
    is_public BOOLEAN := NEW.publication_state = 'PUBLISHED' AND NEW.visibility = 'VISIBLE';
    kind TEXT;
BEGIN
    IF was_public AND NOT is_public THEN
        kind := 'post_gone';
    ELSIF is_public AND (NEW.like_count <> OLD.like_count OR NEW.dislike_count <> OLD.dislike_count) THEN
        kind := 'post_reactions';
    END IF;
    IF kind IS NOT NULL THEN
        PERFORM pg_notify('forum_events', json_build_object(
            'type', kind,
            'post', NEW.id,
            'likes', NEW.like_count,
            'dislikes', NEW.dislike_count
        )::text);
    END IF;
    RETURN NULL;
END $$;

CREATE TRIGGER forum_post_events
AFTER UPDATE ON forum_posts
FOR EACH ROW EXECUTE FUNCTION forum_post_events();
