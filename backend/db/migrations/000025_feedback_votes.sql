-- 000025: FREE comment validity votes and tallies (P14-T04, B-BR-F05/F06).
-- At most one live vote per account/comment/revision: change rewrites
-- the choice in place, removal tombstones (idempotent no-op when
-- absent). Comment edits open a new revision with a fresh denominator;
-- old votes stay restricted audit history. Tallies carry exact V/I
-- counts per revision and rebuild from live rows at any time; the
-- percentage shown is floor(10000*V/total), null on zero votes.
-- Append-only; moderation states arrive in P14-T05 without rewriting.
CREATE TABLE feedback_votes (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts (id),
    comment_id UUID NOT NULL REFERENCES feedback_comments (id) ON DELETE RESTRICT,
    comment_revision INT NOT NULL CONSTRAINT feedback_votes_revision_check CHECK (comment_revision >= 1),
    choice TEXT NOT NULL CONSTRAINT feedback_votes_choice_check CHECK (choice IN ('VALID', 'INVALID')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX feedback_votes_current_unique
    ON feedback_votes (account_id, comment_id, comment_revision)
    WHERE deleted_at IS NULL;
CREATE INDEX feedback_votes_tally_idx
    ON feedback_votes (comment_id, comment_revision)
    WHERE deleted_at IS NULL;
CREATE TABLE feedback_tallies (
    comment_id UUID NOT NULL REFERENCES feedback_comments (id) ON DELETE RESTRICT,
    revision INT NOT NULL CONSTRAINT feedback_tallies_revision_check CHECK (revision >= 1),
    valid_count BIGINT NOT NULL DEFAULT 0,
    invalid_count BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT feedback_tallies_pk PRIMARY KEY (comment_id, revision)
);
