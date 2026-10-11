-- name: NextTrainingErasureCopy :one
SELECT d.* FROM privacy_training_derivatives d
LEFT JOIN privacy_training_erasure_cursors c ON c.contributor_id = d.contributor_id
WHERE d.contributor_id = @contributor_id
AND (d.image_id,d.consent_revision) > (COALESCE(c.last_image_id,''),COALESCE(c.last_revision,0))
ORDER BY d.image_id,d.consent_revision LIMIT 1;

-- name: ClaimTrainingErasureCopy :execrows
UPDATE privacy_training_derivatives d SET
 state = 'REVOKED',
 revoked_at = COALESCE(d.revoked_at,(SELECT r.revoked_at FROM privacy_training_revocations r
 WHERE r.contributor_id = d.contributor_id AND r.revision >= d.consent_revision ORDER BY r.revision LIMIT 1)),
 output_state = 'PURGING', output_nonce = @nonce, output_lease_until = @lease_until
WHERE d.image_id = @image_id AND d.consent_revision = @consent_revision AND d.contributor_id = @contributor_id
AND COALESCE(d.output_lease_until,'-infinity'::timestamptz) <= @at
AND EXISTS (SELECT 1 FROM privacy_training_consents s WHERE s.contributor_id = d.contributor_id AND s.state = 'REVOKED')
AND EXISTS (SELECT 1 FROM privacy_training_revocations r WHERE r.contributor_id = d.contributor_id AND r.revision >= d.consent_revision);

-- name: AdvanceTrainingErasureCursor :exec
INSERT INTO privacy_training_erasure_cursors(contributor_id,last_image_id,last_revision,updated_at)
VALUES (@contributor_id,@image_id,@consent_revision,@at)
ON CONFLICT (contributor_id) DO UPDATE SET last_image_id = EXCLUDED.last_image_id,last_revision = EXCLUDED.last_revision,updated_at = EXCLUDED.updated_at;

-- name: DeleteTrainingErasureCursor :exec
DELETE FROM privacy_training_erasure_cursors WHERE contributor_id = @contributor_id;
