-- EMP-INTEG-006: retained reviews and delivery outcomes for scoped fleet events.
CREATE TABLE org_webhook_endpoints (
    org_id UUID NOT NULL,
    approval_id UUID NOT NULL,
    approval_revision BIGINT NOT NULL,
    endpoint TEXT NOT NULL CHECK (octet_length(endpoint) BETWEEN 12 AND 1024),
    PRIMARY KEY (org_id,approval_id,approval_revision),
    FOREIGN KEY (org_id,approval_id,approval_revision) REFERENCES org_connector_approvals(org_id,id,revision)
);
CREATE TABLE org_webhook_deliveries (
    org_id UUID NOT NULL,
    id UUID NOT NULL,
    connection_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    job_id UUID NOT NULL UNIQUE REFERENCES job_queue(id),
    connection_revision BIGINT NOT NULL CHECK (connection_revision>0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    last_lease BIGINT NOT NULL DEFAULT 0 CHECK (last_lease>=0),
    outcome TEXT NOT NULL DEFAULT 'PENDING' CHECK (outcome IN ('PENDING','UNKNOWN','DELIVERED','RETRY','FAILED','SUPPRESSED')),
    PRIMARY KEY (org_id,id),
    UNIQUE (org_id,connection_id,operation_id),
    FOREIGN KEY (org_id,connection_id) REFERENCES org_connections(org_id,id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_asset_lifecycle_events(org_id,operation_id)
);
DO $$
DECLARE relation TEXT;
BEGIN
    FOREACH relation IN ARRAY ARRAY['org_webhook_endpoints','org_webhook_deliveries'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
        EXECUTE format('CREATE POLICY %I ON %I USING (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',
            relation||'_scope',relation);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
    END LOOP;
END $$;
CREATE TRIGGER org_webhook_endpoints_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_webhook_endpoints
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
CREATE TRIGGER org_webhook_deliveries_retained BEFORE DELETE OR TRUNCATE ON org_webhook_deliveries
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
CREATE FUNCTION org_webhook_delivery_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.attempts<>0 OR NEW.last_lease<>0 OR NEW.outcome<>'PENDING' OR NOT EXISTS (
            SELECT 1 FROM public.org_asset_lifecycle_events l
            JOIN public.org_assets asset ON (asset.org_id,asset.id)=(l.org_id,l.asset_id)
            JOIN public.org_connections c ON (c.org_id,c.id,c.revision)=(NEW.org_id,NEW.connection_id,NEW.connection_revision)
            JOIN public.org_connector_approvals a ON (a.org_id,a.id,a.revision)=(c.org_id,c.approval_id,c.approval_revision)
            JOIN public.org_webhook_endpoints e ON (e.org_id,e.approval_id,e.approval_revision)=(a.org_id,a.id,a.revision)
            WHERE (l.org_id,l.operation_id)=(NEW.org_id,NEW.operation_id)
              AND l.command IN ('DEACTIVATE','REACTIVATE') AND asset.kind='VEHICLE'
              AND a.resource_id=l.asset_id AND a.kind='OUTBOUND_WEBHOOK' AND a.decision='APPROVED'
              AND a.ready_at<=l.recorded_at AND c.state='ACTIVE' AND c.changed_at<=l.recorded_at
              AND NOT EXISTS (SELECT 1 FROM public.org_connector_approvals n
                 WHERE (n.org_id,n.id)=(a.org_id,a.id) AND n.revision>a.revision)
        ) THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='webhook delivery source unavailable';
        END IF;
    ELSIF (NEW.org_id,NEW.id,NEW.connection_id,NEW.operation_id,NEW.job_id,NEW.connection_revision)
          IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.connection_id,OLD.operation_id,OLD.job_id,OLD.connection_revision)
       OR NOT ((OLD.outcome IN ('PENDING','RETRY','UNKNOWN') AND NEW.outcome='UNKNOWN' AND NEW.attempts=OLD.attempts+1 AND NEW.last_lease>OLD.last_lease)
            OR (OLD.outcome='UNKNOWN' AND NEW.outcome IN ('DELIVERED','RETRY','FAILED','SUPPRESSED') AND NEW.attempts=OLD.attempts AND NEW.last_lease=OLD.last_lease)) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid webhook delivery transition';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_webhook_delivery_proof BEFORE INSERT OR UPDATE ON org_webhook_deliveries
    FOR EACH ROW EXECUTE FUNCTION org_webhook_delivery_proof();
