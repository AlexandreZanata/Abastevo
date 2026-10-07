-- Owner/key/station-bound capture permission. Exact fixes are never retained.
CREATE TABLE community_photo_captures (
    id uuid PRIMARY KEY,
    contributor_ref text NOT NULL,
    key_id text NOT NULL,
    client_capture_id text NOT NULL CHECK (length(client_capture_id) BETWEEN 1 AND 128),
    station_id uuid NOT NULL REFERENCES directory_stations(id),
    issued_at timestamptz NOT NULL,
    camera_expires_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    policy_version text NOT NULL CHECK (policy_version = 'photo-capture-v1'),
    evidence_session_id uuid,
    captured_at timestamptz,
    UNIQUE (contributor_ref, client_capture_id),
    CHECK (camera_expires_at > issued_at AND camera_expires_at <= issued_at + interval '2 minutes'),
    CHECK (expires_at > camera_expires_at AND expires_at <= issued_at + interval '24 hours'),
    CHECK ((evidence_session_id IS NULL) = (captured_at IS NULL)),
    CHECK (captured_at IS NULL OR (captured_at >= issued_at AND captured_at < camera_expires_at))
);
CREATE INDEX community_photo_captures_expiry ON community_photo_captures(expires_at);
