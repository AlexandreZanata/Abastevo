-- 000027: feedback comment visibility lane (P14-T05A). Moderator-
-- driven visibility beside the author tombstone: visible (default)
-- and flagged read normally; hidden rows vanish from public reads
-- while staying for audit. Author deletion keeps using deleted_at.
-- Append-only, no history rewrite.
ALTER TABLE feedback_comments ADD COLUMN visibility TEXT NOT NULL DEFAULT 'visible'
    CONSTRAINT feedback_comments_visibility_check CHECK (visibility IN
        ('visible', 'flagged', 'hidden'));
CREATE INDEX feedback_comments_visible_idx
    ON feedback_comments (station_id, product, visibility, created_at, id)
    WHERE deleted_at IS NULL;
