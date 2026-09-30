-- 000024: FREE station/fuel comments and replies (P14-T03, B-BR-F03/F04).
-- Plain text, nonempty, 280 scalars; replies attach to a comment of
-- the same station/fuel at depth one (enforced by CHECK plus
-- application parent validation). Edits bump the revision with
-- compare-and-swap; deletes tombstone (history stays for audit and
-- moderation, reads ignore it). Only the author row links identity;
-- public reads resolve the opaque account alias, never addresses or
-- provider subjects. Append-only; moderation states arrive in P14-T05
-- without rewriting this file.
CREATE TABLE feedback_comments (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    product TEXT NOT NULL CONSTRAINT feedback_comments_product_check CHECK (product <> ''),
    parent_id UUID REFERENCES feedback_comments (id),
    depth SMALLINT NOT NULL DEFAULT 0 CONSTRAINT feedback_comments_depth_check CHECK (depth IN (0, 1)),
    text TEXT NOT NULL CONSTRAINT feedback_comments_text_check CHECK (text <> ''),
    revision INT NOT NULL DEFAULT 1 CONSTRAINT feedback_comments_revision_check CHECK (revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT feedback_comments_reply_check CHECK (
        (parent_id IS NULL AND depth = 0) OR (parent_id IS NOT NULL AND depth = 1)
    )
);
CREATE INDEX feedback_comments_target_idx
    ON feedback_comments (station_id, product, created_at, id)
    WHERE deleted_at IS NULL;
CREATE INDEX feedback_comments_replies_idx
    ON feedback_comments (parent_id, created_at, id)
    WHERE deleted_at IS NULL;
