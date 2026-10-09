-- Immutable source reference for an explicitly reviewed per-product subset.
-- Receipts are ephemeral; retained observation facts must not prevent their purge.
ALTER TABLE community_observations ADD COLUMN photo_capture_id uuid;
CREATE UNIQUE INDEX community_photo_product_unique ON community_observations
 (photo_capture_id, fuel_product, condition_kind, qualifier_key) WHERE photo_capture_id IS NOT NULL;
ALTER TABLE community_observations ADD CONSTRAINT community_photo_requires_evidence
 CHECK (photo_capture_id IS NULL OR (evidence_id IS NOT NULL AND claimed_captured_at IS NOT NULL));
