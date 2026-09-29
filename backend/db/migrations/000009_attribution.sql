-- 000009: contributor attribution tokens (P04-T04). Append-only.
-- Observations reference a random attribution token, never the contributor
-- UUID, so public reads cannot pivot from content to identity rows.
-- Tokens are minted lazily on first submit and never rotate.
ALTER TABLE identity_contributors ADD COLUMN attribution_token TEXT;
CREATE UNIQUE INDEX identity_contributors_token_unique
    ON identity_contributors (attribution_token) WHERE attribution_token IS NOT NULL;
