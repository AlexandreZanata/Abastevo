-- Inventory is committed before final bytes leave the worker. A content key
-- has one owning session and one authority; read-only duplicate reuse never
-- overwrites the first copy or extends its conservative deadline.
CREATE TABLE evidence_final_outputs (
    final_key text PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES evidence_sessions(id) ON DELETE RESTRICT,
    sanitized_sha256 text NOT NULL,
    state text NOT NULL CHECK (state IN ('PENDING','WRITING','READY','PURGING','PURGED')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    lease_nonce uuid,
    lease_until timestamptz,
    deadline_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    purged_at timestamptz,
    CHECK ((lease_nonce IS NULL) = (lease_until IS NULL)),
    CHECK (state NOT IN ('WRITING','PURGING') OR lease_nonce IS NOT NULL)
);
CREATE INDEX evidence_final_output_session_idx ON evidence_final_outputs(session_id);
CREATE INDEX evidence_final_output_pending_idx
    ON evidence_final_outputs (deadline_at, final_key) WHERE state != 'PURGED';

INSERT INTO evidence_final_outputs
    (final_key, session_id, sanitized_sha256, state, deadline_at, created_at, purged_at)
SELECT o.final_key, o.session_id, o.sanitized_sha256,
       CASE WHEN o.final_deleted_at IS NULL THEN 'READY' ELSE 'PURGED' END,
       LEAST(COALESCE(o.received_at, o.created_at), s.created_at) + interval '24 hours',
       LEAST(o.created_at, s.created_at), o.final_deleted_at
FROM evidence_objects o JOIN evidence_sessions s ON s.id = o.session_id;

-- Old verifier crashes may have left bytes without an object row. The
-- verified key was deterministically f/<claimed SHA>; conservatively retain
-- these possible destinations too, without displacing an existing owner.
INSERT INTO evidence_final_outputs
    (final_key, session_id, sanitized_sha256, state, deadline_at, created_at)
SELECT DISTINCT ON (lower(claimed_sha256)) 'f/' || lower(claimed_sha256),
       id, '', 'PENDING', created_at + interval '24 hours', created_at
FROM evidence_sessions
WHERE status IN ('VERIFYING','REJECTED','EXPIRED')
  AND claimed_sha256 ~ '^[0-9a-fA-F]{64}$'
ORDER BY lower(claimed_sha256), created_at, id
ON CONFLICT (final_key) DO NOTHING;
