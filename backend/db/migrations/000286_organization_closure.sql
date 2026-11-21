-- EMP-ORG-013: owner-scheduled closure and a retained retention handoff.
CREATE TABLE org_closure_requests (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    operation_id UUID NOT NULL,
    job_id UUID NOT NULL UNIQUE REFERENCES job_queue(id),
    archive_revision BIGINT NOT NULL CHECK (archive_revision>1),
    requested_at TIMESTAMPTZ NOT NULL CHECK (isfinite(requested_at)),
    closes_at TIMESTAMPTZ NOT NULL CHECK (isfinite(closes_at)),
    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','CANCELLED','COMPLETED')),
    resolved_at TIMESTAMPTZ CHECK (isfinite(resolved_at)),
    cancelled_by_operation UUID,
    hold_count BIGINT CHECK (hold_count>=0),
    PRIMARY KEY (org_id,operation_id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id),
    FOREIGN KEY (org_id,cancelled_by_operation) REFERENCES org_audit_events(org_id,operation_id),
    CHECK (closes_at-requested_at BETWEEN interval '168 hours' AND interval '2160 hours'),
    CHECK ((state='PENDING' AND resolved_at IS NULL AND cancelled_by_operation IS NULL AND hold_count IS NULL)
        OR (state='CANCELLED' AND resolved_at IS NOT NULL AND resolved_at>=requested_at AND resolved_at<closes_at AND cancelled_by_operation IS NOT NULL AND hold_count IS NULL)
        OR (state='COMPLETED' AND resolved_at IS NOT NULL AND resolved_at>=closes_at AND cancelled_by_operation IS NULL AND hold_count IS NOT NULL))
);
CREATE UNIQUE INDEX org_closure_requests_pending ON org_closure_requests(org_id) WHERE state='PENDING';
CREATE TABLE org_lifecycle_receipts (
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    result JSONB NOT NULL CHECK (jsonb_typeof(result)='object'),
    PRIMARY KEY (org_id,operation_id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id)
);
DO $$
DECLARE relation TEXT;
BEGIN
    FOREACH relation IN ARRAY ARRAY['org_closure_requests','org_lifecycle_receipts'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
        EXECUTE format('CREATE POLICY %I ON %I USING (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',
            relation||'_scope',relation);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
    END LOOP;
END $$;
CREATE TRIGGER org_lifecycle_receipts_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON org_lifecycle_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_closure_requests_retained BEFORE DELETE OR TRUNCATE ON org_closure_requests
    FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_closure_request_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_revision BIGINT;
BEGIN
    SELECT revision INTO root_revision FROM public.organizations WHERE org_id=NEW.org_id AND state='ARCHIVED' FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='archived organization required';
    END IF;
    IF TG_OP='INSERT' THEN
        IF NEW.state<>'PENDING' OR NEW.archive_revision<>root_revision OR NEW.requested_at>clock_timestamp()
           OR NOT EXISTS (SELECT 1 FROM public.org_audit_events a WHERE (a.org_id,a.operation_id,a.target_id)=(NEW.org_id,NEW.operation_id,NEW.org_id)
                          AND a.action='org.lifecycle.closure_requested' AND a.target_type='organization.lifecycle')
           OR NOT EXISTS (SELECT 1 FROM public.job_queue j WHERE j.id=NEW.job_id AND j.kind='org.lifecycle.close'
                          AND j.payload->>'org_id'=NEW.org_id::text AND j.payload->>'operation_id'=NEW.operation_id::text
                          AND j.not_before=NEW.closes_at AND j.status='queued') THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='closure request proof unavailable';
        END IF;
    ELSE
        IF OLD.state<>'PENDING' OR (NEW.org_id,NEW.operation_id,NEW.job_id,NEW.archive_revision,NEW.requested_at,NEW.closes_at)
           IS DISTINCT FROM (OLD.org_id,OLD.operation_id,OLD.job_id,OLD.archive_revision,OLD.requested_at,OLD.closes_at) THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='closure history is retained';
        END IF;
        NEW.resolved_at=clock_timestamp();
        IF NOT ((NEW.state='CANCELLED' AND NEW.resolved_at<NEW.closes_at
                AND EXISTS (SELECT 1 FROM public.org_audit_events a WHERE (a.org_id,a.operation_id,a.target_id)=(NEW.org_id,NEW.cancelled_by_operation,NEW.org_id)
                            AND a.action='org.lifecycle.closure_cancelled' AND a.target_type='organization.lifecycle'))
             OR (NEW.state='COMPLETED' AND NEW.resolved_at>=NEW.closes_at
                AND EXISTS (SELECT 1 FROM public.job_queue j WHERE j.id=NEW.job_id AND j.status='leased' AND j.lease_expires_at>NEW.resolved_at))) THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='closure resolution proof unavailable';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_closure_request_guard BEFORE INSERT OR UPDATE ON org_closure_requests
    FOR EACH ROW EXECUTE FUNCTION org_closure_request_guard();

-- A proven terminal closure ends the last owner along with every other member.
CREATE OR REPLACE FUNCTION org_membership_access_policy(org UUID,member UUID,action TEXT) RETURNS TEXT
 LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog AS $$
BEGIN
 IF org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RETURN 'UNAVAILABLE'; END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=org FOR UPDATE;
 IF NOT FOUND THEN RETURN 'UNAVAILABLE'; END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=org AND id=member) THEN RETURN 'UNAVAILABLE'; END IF;
 IF action='ACTIVATE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=org AND r.membership_id=member
   AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp())
   AND (r.ended_at IS NULL OR r.ended_at>clock_timestamp())) THEN RETURN 'UNAVAILABLE'; END IF;
  IF EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=org AND membership_id=member AND role='OWNER')
   AND NOT public.org_access_owner_coverage(org,NULL,NULL,member) THEN RETURN 'LAST_OWNER'; END IF;
  RETURN 'ALLOW';
 ELSIF action='REDUCE' THEN
  IF EXISTS(SELECT 1 FROM public.organizations o JOIN public.org_closure_requests c ON c.org_id=o.org_id
             WHERE o.org_id=org AND o.state='CLOSED' AND c.state='COMPLETED') THEN RETURN 'ALLOW'; END IF;
  IF EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=org AND membership_id=member AND role='OWNER')
   AND NOT public.org_access_owner_coverage(org,member,NULL,NULL) THEN RETURN 'LAST_OWNER'; END IF;
  RETURN 'ALLOW';
 END IF;
 RETURN 'UNAVAILABLE';
END $$;
