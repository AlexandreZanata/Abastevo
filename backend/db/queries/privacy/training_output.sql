-- name: ClaimTrainingOutput :execrows
UPDATE privacy_training_derivatives SET output_state = 'WRITING', output_nonce = @nonce,
output_lease_until = @lease_until, output_attempts = output_attempts + 1
WHERE image_id = @image_id AND consent_revision = @consent_revision
AND state = 'ACTIVE' AND output_state IN ('PENDING','WRITING','READY') AND output_attempts < 5
AND (output_lease_until IS NULL OR output_lease_until <= @at);

-- Cleanup must pin the exact writer before deleting. An old writer cannot
-- remove a new writer's bytes or a completed READY output.
-- name: ClaimTrainingOutputCleanup :execrows
UPDATE privacy_training_derivatives SET output_state = 'PURGING', output_lease_until = @lease_until
WHERE image_id = @image_id AND consent_revision = @consent_revision AND output_nonce = @nonce;

-- name: FinishTrainingOutput :execrows
UPDATE privacy_training_derivatives SET
output_state = CASE WHEN @succeeded::boolean THEN 'READY' WHEN state = 'REVOKED' THEN 'PURGED' ELSE 'PENDING' END,
output_ready_at = CASE WHEN @succeeded::boolean THEN @at::timestamptz ELSE output_ready_at END,
output_purged_at = CASE WHEN NOT @succeeded::boolean AND state = 'REVOKED' THEN @at::timestamptz ELSE output_purged_at END,
output_nonce = NULL, output_lease_until = NULL
WHERE image_id = @image_id AND consent_revision = @consent_revision AND output_nonce = @nonce
AND output_lease_until > @at AND (NOT @succeeded::boolean OR (state = 'ACTIVE' AND output_state = 'WRITING'));
