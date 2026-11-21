-- EMP-INTEG-001: reviewed connector bindings and scoped connection lifecycle.
CREATE TABLE org_connector_approvals (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    revision BIGINT NOT NULL CHECK (revision>0),
    kind TEXT NOT NULL CHECK (kind IN ('TELEMATICS','FUEL_CARD_STATEMENT','FISCAL_LOOKUP','DOCUMENT_EXTRACTION','OUTBOUND_WEBHOOK','ACCOUNTING_EXPORT')),
    provider_code TEXT NOT NULL CHECK (octet_length(provider_code) BETWEEN 1 AND 64 AND provider_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    resource_id UUID,
    credential_ref UUID,
    maximum_bytes BIGINT NOT NULL CHECK (maximum_bytes > 0),
    maximum_items INTEGER NOT NULL CHECK (maximum_items > 0),
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED','WITHDRAWN')),
    decision_ref TEXT NOT NULL CHECK (octet_length(decision_ref) BETWEEN 1 AND 200 AND decision_ref ~ '^[!-~]+$'),
    ready_at TIMESTAMPTZ CHECK (isfinite(ready_at)),
    PRIMARY KEY (org_id,id,revision),
    CHECK (credential_ref IS NULL OR credential_ref <> '00000000-0000-0000-0000-000000000000'::uuid),
    CHECK (resource_id IS NULL OR resource_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CHECK (decision='APPROVED' OR ready_at IS NULL)
);

CREATE TABLE org_connections (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    approval_id UUID NOT NULL,
    approval_revision BIGINT NOT NULL,
    label TEXT NOT NULL CHECK (octet_length(label) BETWEEN 1 AND 100 AND label=btrim(label) AND label !~ '[[:cntrl:]]'),
    state TEXT NOT NULL CHECK (state IN ('CONFIGURED','ACTIVE','DEGRADED','DISABLED')),
    revision BIGINT NOT NULL CHECK (revision > 0),
    changed_at TIMESTAMPTZ NOT NULL CHECK (isfinite(changed_at)),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,approval_id,approval_revision),
    FOREIGN KEY(org_id,approval_id,approval_revision) REFERENCES org_connector_approvals(org_id,id,revision)
);

ALTER TABLE org_connector_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_connector_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY org_connector_approvals_scope ON org_connector_approvals
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_connections FORCE ROW LEVEL SECURITY;
CREATE POLICY org_connections_scope ON org_connections
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

CREATE TRIGGER org_connector_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_connector_approvals
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
CREATE TRIGGER org_connections_keep_history BEFORE DELETE OR TRUNCATE ON org_connections
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();

CREATE FUNCTION org_connection_retain_scope() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF (NEW.org_id,NEW.id,NEW.approval_id,NEW.approval_revision,NEW.label)
           IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.approval_id,OLD.approval_revision,OLD.label)
       OR NEW.revision<>OLD.revision+1 OR NEW.changed_at<OLD.changed_at
       OR NOT ((OLD.state IN ('CONFIGURED','DEGRADED','DISABLED') AND NEW.state='ACTIVE')
            OR (OLD.state='ACTIVE' AND NEW.state='DEGRADED')
            OR (OLD.state<>'DISABLED' AND NEW.state='DISABLED')) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid organization connection transition';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_connection_state_proof BEFORE UPDATE ON org_connections
    FOR EACH ROW EXECUTE FUNCTION org_connection_retain_scope();
REVOKE ALL ON org_connector_approvals,org_connections FROM PUBLIC;

CREATE FUNCTION org_connector_review_order() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE previous BIGINT;
BEGIN
    PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id FOR KEY SHARE;
    PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended(
        'org-connector-approval:' || NEW.org_id::text || ':' || NEW.id::text,0));
    SELECT COALESCE(max(revision),0) INTO previous FROM public.org_connector_approvals
    WHERE org_id=NEW.org_id AND id=NEW.id;
    IF NEW.revision<>previous+1 THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='noncontiguous connector approval revision';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_connector_approval_order BEFORE INSERT ON org_connector_approvals
    FOR EACH ROW EXECUTE FUNCTION org_connector_review_order();
