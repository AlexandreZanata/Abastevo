-- 000028: feedback account-footprint indexes (P14-T05B, B-BR-F02/F07).
-- Export and erasure list one account's ratings, comments and votes.
-- These are rare background/operator operations, not hot paths; the
-- indexes keep them bounded without changing write semantics.
-- Append-only, no history rewrite.
CREATE INDEX IF NOT EXISTS feedback_ratings_account_idx
    ON feedback_ratings (account_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS feedback_comments_account_idx
    ON feedback_comments (account_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS feedback_votes_account_idx
    ON feedback_votes (account_id)
    WHERE deleted_at IS NULL;
