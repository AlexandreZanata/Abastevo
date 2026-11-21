-- EMP-DATA-002: scoped Data verifiers and current rotation state.
CREATE TABLE org_data_api_keys (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    label TEXT NOT NULL CHECK (octet_length(label) BETWEEN 1 AND 100 AND btrim(label) <> ''),
    scopes TEXT[] NOT NULL CHECK (scopes = ARRAY['regional.read']::text[]),
    verifier TEXT NOT NULL CHECK (verifier ~ '^[a-f0-9]{64}$'),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE','ROTATING','REVOKED')),
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    issued_at TIMESTAMPTZ NOT NULL CHECK (isfinite(issued_at)),
    expires_at TIMESTAMPTZ NOT NULL CHECK (isfinite(expires_at) AND expires_at > issued_at),
    overlap_until TIMESTAMPTZ,
    replacement_id UUID,
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (org_id,id),
    FOREIGN KEY (org_id,replacement_id) REFERENCES org_data_api_keys(org_id,id),
    CHECK (replacement_id IS NULL OR replacement_id <> id),
    CHECK ((overlap_until IS NULL AND replacement_id IS NULL) OR
           (overlap_until IS NOT NULL AND replacement_id IS NOT NULL AND isfinite(overlap_until) AND overlap_until <= expires_at)),
    CHECK ((state='ACTIVE' AND replacement_id IS NULL AND revoked_at IS NULL) OR
           (state='ROTATING' AND replacement_id IS NOT NULL AND revoked_at IS NULL) OR
           (state='REVOKED' AND revoked_at IS NOT NULL AND isfinite(revoked_at) AND revoked_at >= issued_at))
);

ALTER TABLE org_data_api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_api_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY org_data_api_keys_scope ON org_data_api_keys
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_data_api_keys_no_delete BEFORE DELETE OR TRUNCATE ON org_data_api_keys
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();

CREATE FUNCTION org_data_key_protect_terms() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF (NEW.org_id,NEW.id,NEW.label,NEW.scopes,NEW.verifier,NEW.issued_at,NEW.expires_at)
        IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.label,OLD.scopes,OLD.verifier,OLD.issued_at,OLD.expires_at)
       OR NEW.revision <> OLD.revision+1
       OR NOT ((OLD.state='ACTIVE' AND NEW.state IN ('ROTATING','REVOKED'))
            OR (OLD.state='ROTATING' AND NEW.state='REVOKED'
                AND NEW.overlap_until=OLD.overlap_until AND NEW.replacement_id=OLD.replacement_id)) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid Data key state transition';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_data_key_terms BEFORE UPDATE ON org_data_api_keys
    FOR EACH ROW EXECUTE FUNCTION org_data_key_protect_terms();
REVOKE ALL ON org_data_api_keys FROM PUBLIC;
