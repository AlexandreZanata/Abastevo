-- RVA07: hot cleanup excludes purged/healthy READY/unused pending inventory.
CREATE INDEX privacy_training_cleanup_due_idx
 ON privacy_training_derivatives(COALESCE(output_lease_until, '-infinity'::timestamptz), promoted_at, image_id, consent_revision)
 WHERE output_state != 'PURGED'
 AND (state = 'REVOKED' OR output_state IN ('WRITING','PURGING') OR output_attempts >= 5);
