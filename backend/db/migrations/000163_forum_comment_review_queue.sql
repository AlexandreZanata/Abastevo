-- 000163: Forum comment review queue (B-BR-FOR-09). Append-only.
-- Operators approve or reject PENDING_REVIEW comments oldest first; the
-- partial index keeps the queue scan bounded to unreviewed rows.
CREATE INDEX forum_comments_review_queue_idx
    ON forum_comments (created_at, id)
    WHERE publication_state = 'PENDING_REVIEW';
