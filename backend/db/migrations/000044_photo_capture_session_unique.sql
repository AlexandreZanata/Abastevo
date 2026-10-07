-- One operational media session belongs to one immutable capture receipt.
CREATE UNIQUE INDEX community_photo_capture_session_unique
 ON community_photo_captures(evidence_session_id) WHERE evidence_session_id IS NOT NULL;
