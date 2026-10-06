-- 000038: claim proof metadata (P30-T04). Append-only: one row per
-- accepted proof version, hash-bound and immutable. Exactly one proof
-- binds per declaration (partial unique index); bytes live in private
-- object storage behind server-generated keys (never PostgreSQL, never
-- Git/logs). Expiry/deletion is a lifecycle, not a delete: rows stay
-- as audit with terminal states while bytes purge.
CREATE TABLE claim_proofs (
    id UUID PRIMARY KEY,
    claim_id UUID NOT NULL REFERENCES profile_claims (id) ON DELETE RESTRICT,
    declaration_id UUID NOT NULL REFERENCES claim_declarations (id) ON DELETE RESTRICT,
    sha256 TEXT NOT NULL,
    bytes_size BIGINT NOT NULL,
    format TEXT NOT NULL DEFAULT 'application/pdf',
    evidence_kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'received',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT claim_proofs_kind_check
        CHECK (evidence_kind IN ('authorization', 'mandate', 'scan')),
    CONSTRAINT claim_proofs_status_check
        CHECK (status IN ('received', 'verified', 'rejected', 'expired', 'deleted'))
);
CREATE UNIQUE INDEX claim_proofs_declaration_unique
    ON claim_proofs (declaration_id) WHERE status NOT IN ('rejected', 'deleted');
CREATE INDEX claim_proofs_expiry_idx
    ON claim_proofs (expires_at) WHERE status NOT IN ('expired', 'deleted');
