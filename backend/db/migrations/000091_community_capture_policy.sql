-- Preserve v1/debug receipts and immutable deadlines; new normal policy has no location requirement.
ALTER TABLE community_photo_captures DROP CONSTRAINT community_photo_captures_policy_version_check;
ALTER TABLE community_photo_captures ADD CONSTRAINT community_photo_captures_policy_version_check
 CHECK (policy_version IN ('photo-capture-v1', 'photo-capture-ui-test-v1', 'photo-capture-community-v2'));
