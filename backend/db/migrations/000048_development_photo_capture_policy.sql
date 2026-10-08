-- Temporary UI-validation provenance is distinct from verified on-site capture.
ALTER TABLE community_photo_captures DROP CONSTRAINT community_photo_captures_policy_version_check;
ALTER TABLE community_photo_captures ADD CONSTRAINT community_photo_captures_policy_version_check
 CHECK (policy_version IN ('photo-capture-v1', 'photo-capture-ui-test-v1'));
