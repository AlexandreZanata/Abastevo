-- RVA05: immutable output locator and recoverable cross-resource state.
ALTER TABLE privacy_training_derivatives
 ADD COLUMN object_key TEXT,
 ADD COLUMN output_state TEXT NOT NULL DEFAULT 'PENDING' CHECK(output_state IN ('PENDING','WRITING','READY','PURGING','PURGED')),
 ADD COLUMN output_nonce UUID,
 ADD COLUMN output_lease_until TIMESTAMPTZ,
 ADD COLUMN output_attempts INTEGER NOT NULL DEFAULT 0 CHECK(output_attempts BETWEEN 0 AND 5),
 ADD COLUMN output_ready_at TIMESTAMPTZ,
 ADD COLUMN output_purged_at TIMESTAMPTZ,
 ADD CONSTRAINT training_output_lease_pair CHECK((output_nonce IS NULL) = (output_lease_until IS NULL)),
 ADD CONSTRAINT training_output_ready_receipt CHECK(output_state != 'READY' OR (state = 'ACTIVE' AND output_ready_at IS NOT NULL));
-- Existing inventory does not prove bytes exist. Reconcile before selection.
UPDATE privacy_training_derivatives SET
 object_key = 'training/v1/' || to_char(promoted_at AT TIME ZONE 'UTC','YYYY-MM-DD') || '/' || image_id || '-' || consent_revision::text || '.jpg',
 output_state = CASE WHEN state = 'REVOKED' THEN 'PURGING' ELSE 'PENDING' END;
ALTER TABLE privacy_training_derivatives ALTER COLUMN object_key SET NOT NULL;
ALTER TABLE privacy_training_derivatives ADD CONSTRAINT training_output_key_bound CHECK(char_length(object_key) BETWEEN 1 AND 512);
CREATE INDEX privacy_training_output_pending_idx ON privacy_training_derivatives(output_state, promoted_at, image_id, consent_revision)
 WHERE output_state != 'READY' AND output_state != 'PURGED';
