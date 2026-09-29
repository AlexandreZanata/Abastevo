-- 000004: anonymous identity (P03-T02). Append-only.
-- Contributors carry no personal-account columns. Keys bind one public
-- JWK to one contributor through a unique fingerprint. Challenges are
-- consumed atomically exactly once before expiry; a failed proof never
-- consumes, so clients can retry.
CREATE TABLE identity_contributors (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT identity_contributors_status_check
        CHECK (status IN ('active', 'blocked', 'deleted'))
);
CREATE TABLE identity_keys (
    id UUID PRIMARY KEY,
    contributor_id UUID NOT NULL REFERENCES identity_contributors (id) ON DELETE RESTRICT,
    algorithm TEXT NOT NULL,
    public_jwk TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT identity_keys_algorithm_check
        CHECK (algorithm IN ('ecdsa-p256-sha512'))
);
CREATE UNIQUE INDEX identity_keys_fingerprint_unique ON identity_keys (fingerprint);
CREATE INDEX identity_keys_contributor_idx ON identity_keys (contributor_id);
CREATE TABLE identity_challenges (
    id UUID PRIMARY KEY,
    nonce_hash TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    purpose TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    CONSTRAINT identity_challenges_purpose_check
        CHECK (purpose IN ('REGISTER', 'SIGN'))
);
CREATE INDEX identity_challenges_expiry_idx
    ON identity_challenges (expires_at) WHERE consumed_at IS NULL;
