-- EMP-INTEG-002: scoped, coordinate-free receipts for vendor quality intake.
CREATE TABLE org_telematics_evidence (
    org_id UUID NOT NULL,
    connection_id UUID NOT NULL,
    batch_id UUID NOT NULL CHECK(batch_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    trip_id UUID NOT NULL,
    tracking_session_id UUID NOT NULL,
    provider_code TEXT NOT NULL CHECK (octet_length(provider_code) BETWEEN 1 AND 64 AND provider_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[a-f0-9]{64}$'),
    precise INTEGER NOT NULL CHECK (precise >= 0),
    approximate INTEGER NOT NULL CHECK (approximate >= 0),
    insufficient INTEGER NOT NULL CHECK (insufficient >= 0),
    received_at TIMESTAMPTZ NOT NULL CHECK (isfinite(received_at)),
    PRIMARY KEY (org_id,connection_id,batch_id),
    FOREIGN KEY (org_id,connection_id) REFERENCES org_connections(org_id,id),
    FOREIGN KEY (org_id,trip_id) REFERENCES org_trips(org_id,id),
    FOREIGN KEY (org_id,tracking_session_id) REFERENCES org_tracking_sessions(org_id,id),
    CHECK (precise::bigint+approximate+insufficient BETWEEN 1 AND 200)
);
ALTER TABLE org_telematics_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_telematics_evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY org_telematics_evidence_scope ON org_telematics_evidence
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_telematics_evidence_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_telematics_evidence
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
REVOKE ALL ON org_telematics_evidence FROM PUBLIC;
