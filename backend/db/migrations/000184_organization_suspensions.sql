-- EMP-ORG-009: actual operator suspension history and durable owner notices.
-- No runtime role, bypass policy, past interval or subscription source is seeded.
CREATE TABLE org_suspension_periods(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 reason TEXT NOT NULL CHECK(reason IN('BILLING','TERMS','SECURITY','LEGAL','OWNER_REQUEST')),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),ended_at TIMESTAMPTZ,
 started_revision BIGINT NOT NULL CHECK(started_revision>1),ended_revision BIGINT,
 started_session_id UUID NOT NULL REFERENCES admin_sessions(id),ended_session_id UUID REFERENCES admin_sessions(id),
 PRIMARY KEY(org_id,id),UNIQUE(org_id,started_revision),
 CHECK(isfinite(started_at)),CHECK(ended_at IS NULL OR(isfinite(ended_at) AND ended_at>=started_at)),
 CHECK((ended_at IS NULL AND ended_revision IS NULL AND ended_session_id IS NULL) OR(ended_at IS NOT NULL AND ended_revision>started_revision AND ended_session_id IS NOT NULL))
);
CREATE UNIQUE INDEX org_suspension_one_open ON org_suspension_periods(org_id) WHERE ended_at IS NULL;
CREATE INDEX org_suspension_intake_history ON org_suspension_periods(org_id,started_at,ended_at);
CREATE TABLE org_suspension_notices(
 org_id UUID NOT NULL,root_revision BIGINT NOT NULL CHECK(root_revision>1),period_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('CAUSE_OPENED','CAUSE_RESOLVED')),
 audience TEXT NOT NULL DEFAULT 'OWNERS' CHECK(audience='OWNERS'),
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,root_revision),FOREIGN KEY(org_id,period_id) REFERENCES org_suspension_periods(org_id,id)
);
ALTER TABLE org_suspension_periods ENABLE ROW LEVEL SECURITY;ALTER TABLE org_suspension_periods FORCE ROW LEVEL SECURITY;
ALTER TABLE org_suspension_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_suspension_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_suspension_period_scope ON org_suspension_periods USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_suspension_notice_scope ON org_suspension_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_suspension_periods,org_suspension_notices FROM PUBLIC;
CREATE TRIGGER org_suspension_keep_periods BEFORE DELETE OR TRUNCATE ON org_suspension_periods FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_suspension_keep_notices BEFORE UPDATE OR DELETE OR TRUNCATE ON org_suspension_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_suspension_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_revision BIGINT;root_reason TEXT;root_state TEXT;last_end TIMESTAMPTZ;session_id UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT revision,suspension_reason,state INTO root_revision,root_reason,root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF TG_OP='INSERT' THEN
  IF root_state NOT IN('SUSPENDED','ARCHIVED') OR root_reason IS DISTINCT FROM NEW.reason OR root_revision IS DISTINCT FROM NEW.started_revision
   OR NEW.ended_at IS NOT NULL OR NEW.ended_revision IS NOT NULL OR NEW.ended_session_id IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P8401',MESSAGE='suspension source unavailable';END IF;
  IF EXISTS(SELECT 1 FROM public.org_suspension_periods WHERE org_id=NEW.org_id AND ended_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='P8402',MESSAGE='suspension already open';END IF;
  NEW.started_at=clock_timestamp();SELECT max(ended_at) INTO last_end FROM public.org_suspension_periods WHERE org_id=NEW.org_id;
  IF NEW.started_at<last_end THEN RAISE EXCEPTION USING ERRCODE='P8401',MESSAGE='suspension clock unavailable';END IF;session_id=NEW.started_session_id;
 ELSE
  IF ROW(NEW.org_id,NEW.id,NEW.reason,NEW.started_at,NEW.started_revision,NEW.started_session_id) IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.reason,OLD.started_at,OLD.started_revision,OLD.started_session_id)
   OR OLD.ended_at IS NOT NULL OR NEW.ended_revision IS NULL OR NEW.ended_session_id IS NULL OR NEW.ended_revision<=OLD.started_revision
   OR root_revision IS DISTINCT FROM NEW.ended_revision OR root_state NOT IN('ACTIVE','ARCHIVED') OR root_reason<>'' THEN RAISE EXCEPTION USING ERRCODE='P8401',MESSAGE='suspension resolution unavailable';END IF;
  NEW.ended_at=clock_timestamp();IF NEW.ended_at<OLD.started_at THEN RAISE EXCEPTION USING ERRCODE='P8401',MESSAGE='suspension clock unavailable';END IF;session_id=NEW.ended_session_id;
 END IF;
 PERFORM 1 FROM public.admin_sessions WHERE id=session_id AND revoked_at IS NULL AND created_at<=clock_timestamp() AND created_at>clock_timestamp()-interval '15 minutes' AND last_seen_at>clock_timestamp()-interval '30 minutes' AND expires_at>clock_timestamp();
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P8403',MESSAGE='operator proof required';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_suspension_guard BEFORE INSERT OR UPDATE ON org_suspension_periods FOR EACH ROW EXECUTE FUNCTION org_suspension_guard();
CREATE FUNCTION org_suspension_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR NOT EXISTS(SELECT 1 FROM public.org_suspension_periods p WHERE p.org_id=NEW.org_id AND p.id=NEW.period_id AND((NEW.kind='CAUSE_OPENED' AND p.started_revision=NEW.root_revision) OR(NEW.kind='CAUSE_RESOLVED' AND p.ended_revision=NEW.root_revision))) THEN RAISE EXCEPTION USING ERRCODE='P8401',MESSAGE='suspension notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_suspension_notice_guard BEFORE INSERT ON org_suspension_notices FOR EACH ROW EXECUTE FUNCTION org_suspension_notice_guard();
