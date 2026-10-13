-- name: ClaimTrainingPurge :one
WITH candidate AS (
 SELECT x.image_id, x.consent_revision FROM privacy_training_derivatives x
 WHERE x.output_state != 'PURGED'
 AND (x.state = 'REVOKED' OR x.output_state IN ('WRITING','PURGING') OR x.output_attempts >= 5)
 AND COALESCE(x.output_lease_until, '-infinity'::timestamptz) <= @at
 AND (x.state != 'REVOKED' OR x.revoked_at <= @at)
 ORDER BY COALESCE(x.output_lease_until, '-infinity'::timestamptz), x.promoted_at, x.image_id, x.consent_revision
 LIMIT 1 FOR UPDATE SKIP LOCKED
)
UPDATE privacy_training_derivatives d SET output_state = 'PURGING', output_nonce = @nonce, output_lease_until = @lease_until
FROM candidate c WHERE d.image_id = c.image_id AND d.consent_revision = c.consent_revision
RETURNING d.*;

-- name: CompleteTrainingPurge :execrows
UPDATE privacy_training_derivatives SET
 output_state = CASE WHEN state = 'REVOKED' OR output_attempts >= 5 THEN 'PURGED' ELSE 'PENDING' END,
 output_purged_at = CASE WHEN state = 'REVOKED' OR output_attempts >= 5 THEN @at::timestamptz ELSE output_purged_at END,
 output_nonce = NULL, output_lease_until = NULL
WHERE image_id = @image_id AND consent_revision = @consent_revision
AND output_state = 'PURGING' AND output_nonce = @nonce AND output_lease_until > @at;
