-- Migration 000159_forum_comments.sql: Comments and replies schema, sequence, and constraints.
-- Reconciles B-BR-FOR-04-14, BUC-FOR-04/05/06/09.

-- Add monotonic next_comment_sequence to forum_posts
ALTER TABLE forum_posts
    ADD COLUMN next_comment_sequence BIGINT NOT NULL DEFAULT 1 CHECK (next_comment_sequence >= 1);

-- Expand forum_changes target_type check to include COMMENT
ALTER TABLE forum_changes DROP CONSTRAINT forum_changes_target_type_check;
ALTER TABLE forum_changes ADD CONSTRAINT forum_changes_target_type_check
    CHECK (target_type IN ('POST', 'SOURCE', 'COMMENT'));

-- Create forum_comments table
CREATE TABLE forum_comments (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL REFERENCES forum_posts(id) ON DELETE RESTRICT,
    author_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    public_alias TEXT NOT NULL DEFAULT '',
    parent_id UUID REFERENCES forum_comments(id) ON DELETE RESTRICT,
    depth SMALLINT NOT NULL DEFAULT 0 CHECK (depth IN (0, 1)),
    sequence BIGINT NOT NULL CHECK (sequence >= 1),
    body_text TEXT NOT NULL CHECK (
        octet_length(body_text) <= 1200 AND
        (
            (publication_state = 'REMOVED' AND body_text = '') OR
            (publication_state <> 'REMOVED' AND char_length(trim(both from body_text)) BETWEEN 1 AND 280)
        )
    ),
    revision INT NOT NULL DEFAULT 1 CHECK (revision >= 1),
    publication_state TEXT NOT NULL DEFAULT 'PENDING_REVIEW' CHECK (publication_state IN ('PENDING_REVIEW', 'PUBLISHED', 'REJECTED', 'REMOVED')),
    visibility TEXT NOT NULL DEFAULT 'VISIBLE' CHECK (visibility IN ('VISIBLE', 'HIDDEN')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    create_operation_id UUID NOT NULL,
    body_sha256 TEXT NOT NULL DEFAULT '',
    CONSTRAINT forum_comments_author_op_uniq UNIQUE (author_account_id, create_operation_id),
    CONSTRAINT forum_comments_post_seq_uniq UNIQUE (post_id, sequence),
    CONSTRAINT forum_comments_parent_depth_check CHECK (
        (parent_id IS NULL AND depth = 0) OR
        (parent_id IS NOT NULL AND depth = 1)
    ),
    CONSTRAINT forum_comments_author_state_check CHECK (
        (publication_state <> 'REMOVED' AND author_account_id IS NOT NULL) OR
        (publication_state = 'REMOVED')
    ),
    CONSTRAINT forum_comments_published_date_check CHECK (
        (publication_state = 'PUBLISHED' AND published_at IS NOT NULL) OR
        (publication_state <> 'PUBLISHED')
    )
);

-- Partial index for visible published top-level comments on a post
CREATE INDEX forum_comments_post_seq_idx
    ON forum_comments (post_id, sequence, id)
    WHERE parent_id IS NULL AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

-- Partial index for visible published replies to a parent comment
CREATE INDEX forum_comments_reply_seq_idx
    ON forum_comments (parent_id, sequence, id)
    WHERE parent_id IS NOT NULL AND publication_state = 'PUBLISHED' AND visibility = 'VISIBLE';

-- Index for author comments (for history, live quota counts, and erasure)
CREATE INDEX forum_comments_author_created_idx
    ON forum_comments (author_account_id, created_at, id)
    WHERE author_account_id IS NOT NULL;

-- Index for parent comments
CREATE INDEX forum_comments_parent_id_idx
    ON forum_comments (parent_id);
