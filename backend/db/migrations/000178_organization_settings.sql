-- EMP-ORG-008. Immutable typed settings, serialized server intake and scoped
-- routing references. No unscoped legacy backfill or invented past authoring.
CREATE FUNCTION org_settings_values_valid(v JSONB) RETURNS BOOLEAN
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p JSONB;r JSONB;pair JSONB;item RECORD;k TEXT;raw TEXT;n BIGINT;
BEGIN
 IF v IS NULL OR jsonb_typeof(v)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(v))<>7
  OR NOT v ?& ARRAY['time_zone','consumption_display','review_required','approval_levels_default','closure_period_days','policies','responsibilities'] THEN RETURN false;END IF;
 IF jsonb_typeof(v->'time_zone')<>'string' OR length(v->>'time_zone')>128 OR v->>'time_zone' !~ '^[A-Za-z0-9_+/-]+$'
  OR(v->>'time_zone'<>'UTC' AND position('/' IN(v->>'time_zone'))=0) OR v->>'time_zone' ~ '^(posix/|right/|SystemV/)' OR v->>'time_zone' ~ '(^/|/$|//)'
  OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=v->>'time_zone') THEN RETURN false;END IF;
 IF jsonb_typeof(v->'consumption_display')<>'string' OR v->>'consumption_display' NOT IN('KM_PER_L','L_PER_100KM') THEN RETURN false;END IF;
 r=v->'review_required';IF jsonb_typeof(r)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(r))<>3 OR NOT r ?& ARRAY['trips','fuelings','expenses'] THEN RETURN false;END IF;
 FOR k IN SELECT unnest(ARRAY['trips','fuelings','expenses']) LOOP IF jsonb_typeof(r->k)<>'boolean' THEN RETURN false;END IF;END LOOP;
 FOR k IN SELECT unnest(ARRAY['approval_levels_default','closure_period_days']) LOOP
  IF jsonb_typeof(v->k)<>'number' OR v->>k !~ '^[0-9]{1,2}$' THEN RETURN false;END IF;
 END LOOP;
 IF(v->>'approval_levels_default')::int NOT BETWEEN 1 AND 3 OR(v->>'closure_period_days')::int NOT BETWEEN 7 AND 90 THEN RETURN false;END IF;
 p=v->'policies';IF jsonb_typeof(p)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(p))<>5 OR NOT p ?& ARRAY['intake_window_days','clock_skew_limit_seconds','delegation_maximum_days','invitation_validity_days','activation_validity_hours'] THEN RETURN false;END IF;
 FOR k IN SELECT unnest(ARRAY['intake_window_days','delegation_maximum_days','invitation_validity_days','activation_validity_hours']) LOOP
  IF jsonb_typeof(p->k)<>'number' OR p->>k !~ '^[0-9]{1,3}$' THEN RETURN false;END IF;
 END LOOP;
 IF(p->>'intake_window_days')::int NOT BETWEEN 0 AND 60 OR(p->>'delegation_maximum_days')::int NOT BETWEEN 1 AND 180
  OR(p->>'invitation_validity_days')::int NOT BETWEEN 1 AND 30 OR(p->>'activation_validity_hours')::int NOT BETWEEN 1 AND 168 THEN RETURN false;END IF;
 raw=p->>'clock_skew_limit_seconds';IF jsonb_typeof(p->'clock_skew_limit_seconds')<>'string' OR raw !~ '^(0|[1-9][0-9]{0,18})$' OR(length(raw)=19 AND raw>'9223372036854775807') THEN RETURN false;END IF;
 r=v->'responsibilities';IF jsonb_typeof(r)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(r))<>4 OR NOT r ?& ARRAY['review_fallback','platform_notices','data_protection','billing'] THEN RETURN false;END IF;
 FOR item IN SELECT * FROM jsonb_each(r) LOOP
  pair=item.value;IF jsonb_typeof(pair)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(pair))<>2 OR NOT pair ?& ARRAY['primary','substitute'] THEN RETURN false;END IF;
  FOR k IN SELECT unnest(ARRAY['primary','substitute']) LOOP
   raw=pair->>k;IF jsonb_typeof(pair->k)<>'string' OR(raw<>'' AND(raw !~ '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$' OR raw='00000000-0000-0000-0000-000000000000')) THEN RETURN false;END IF;
  END LOOP;
  IF pair->>'primary'<>'' AND pair->>'primary'=pair->>'substitute' THEN RETURN false;END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN invalid_text_representation OR numeric_value_out_of_range THEN RETURN false;
END $$;
CREATE FUNCTION org_settings_defaults(zone TEXT,creator TEXT) RETURNS JSONB
 LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT jsonb_build_object('time_zone',zone,'consumption_display','KM_PER_L','review_required',jsonb_build_object('trips',true,'fuelings',true,'expenses',true),
 'approval_levels_default',1,'closure_period_days',30,'policies',jsonb_build_object('intake_window_days',15,'clock_skew_limit_seconds','300','delegation_maximum_days',90,'invitation_validity_days',7,'activation_validity_hours',48),
 'responsibilities',jsonb_build_object('review_fallback',jsonb_build_object('primary','','substitute',''),'platform_notices',jsonb_build_object('primary',COALESCE(creator,''),'substitute',''),'data_protection',jsonb_build_object('primary',COALESCE(creator,''),'substitute',''),'billing',jsonb_build_object('primary',COALESCE(creator,''),'substitute','')))
$$;
CREATE TABLE org_settings_versions(
 org_id UUID NOT NULL REFERENCES organizations(org_id),revision BIGINT NOT NULL CHECK(revision>0),
 effective_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),settings_values JSONB NOT NULL,
 source TEXT NOT NULL CHECK(source IN('ONBOARDING','BOOTSTRAP','COMMAND','MEMBERSHIP_REDUCTION')),
 author_account_id UUID REFERENCES accounts(id),author_membership_id UUID,reason TEXT NOT NULL,operation_id UUID,
 source_membership_id UUID,source_membership_revision BIGINT,
 PRIMARY KEY(org_id,revision),FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,source_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(isfinite(effective_at)),CHECK(org_settings_values_valid(settings_values)),
 CHECK((source IN('ONBOARDING','COMMAND') AND author_account_id IS NOT NULL AND author_membership_id IS NOT NULL AND operation_id IS NOT NULL AND source_membership_id IS NULL AND source_membership_revision IS NULL)
  OR(source='BOOTSTRAP' AND revision=1 AND author_account_id IS NULL AND author_membership_id IS NULL AND operation_id IS NULL AND source_membership_id IS NULL AND source_membership_revision IS NULL)
  OR(source='MEMBERSHIP_REDUCTION' AND revision>1 AND author_account_id IS NULL AND author_membership_id IS NULL AND operation_id IS NULL AND source_membership_id IS NOT NULL AND source_membership_revision>1)),
 CHECK(source<>'ONBOARDING' OR revision=1),CHECK(source<>'COMMAND' OR revision>1)
);
CREATE TABLE org_settings_heads(
 org_id UUID PRIMARY KEY REFERENCES organizations(org_id),revision BIGINT NOT NULL,
 FOREIGN KEY(org_id,revision) REFERENCES org_settings_versions(org_id,revision)
);
CREATE TABLE org_settings_designations(
 org_id UUID NOT NULL,settings_revision BIGINT NOT NULL,responsibility TEXT NOT NULL CHECK(responsibility IN('review_fallback','platform_notices','data_protection','billing')),
 slot TEXT NOT NULL CHECK(slot IN('primary','substitute')),member_id UUID NOT NULL,
 PRIMARY KEY(org_id,settings_revision,responsibility,slot),
 FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision),
 FOREIGN KEY(org_id,member_id) REFERENCES org_memberships(org_id,id)
);
CREATE INDEX org_settings_designation_member ON org_settings_designations(org_id,member_id,settings_revision);
CREATE TABLE org_settings_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,actor_id UUID NOT NULL REFERENCES accounts(id),request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 settings_revision BIGINT NOT NULL CHECK(settings_revision>1),result_root_revision BIGINT NOT NULL CHECK(result_root_revision>0),PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid)
);
CREATE TABLE org_settings_notices(
 org_id UUID NOT NULL,settings_revision BIGINT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'DESIGNATION_CLEARED' CHECK(kind='DESIGNATION_CLEARED'),
 audience TEXT NOT NULL DEFAULT 'ADMIN_OWNER' CHECK(audience='ADMIN_OWNER'),
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,settings_revision),FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision)
);
ALTER TABLE org_settings_versions ENABLE ROW LEVEL SECURITY;ALTER TABLE org_settings_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE org_settings_heads ENABLE ROW LEVEL SECURITY;ALTER TABLE org_settings_heads FORCE ROW LEVEL SECURITY;
ALTER TABLE org_settings_designations ENABLE ROW LEVEL SECURITY;ALTER TABLE org_settings_designations FORCE ROW LEVEL SECURITY;
ALTER TABLE org_settings_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_settings_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_settings_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_settings_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_settings_version_scope ON org_settings_versions USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_settings_head_scope ON org_settings_heads USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_settings_designation_scope ON org_settings_designations USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_settings_command_scope ON org_settings_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_settings_notice_scope ON org_settings_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_settings_versions,org_settings_heads,org_settings_designations,org_settings_commands,org_settings_notices FROM PUBLIC;
CREATE TRIGGER org_settings_version_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_settings_versions FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_settings_head_history BEFORE DELETE OR TRUNCATE ON org_settings_heads FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_settings_designation_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_settings_designations FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_settings_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_settings_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_settings_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_settings_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_settings_member_available(selected_org UUID,member UUID) RETURNS BOOLEAN
 LANGUAGE sql VOLATILE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id) JOIN public.accounts a ON a.id=m.account_id
 WHERE m.org_id=selected_org AND m.id=member AND m.state='ACTIVE' AND e.state='ACTIVE' AND a.status='active'
 AND NOT EXISTS(SELECT 1 FROM public.payment_billing_exits x WHERE x.account_id=a.id AND x.reason='ACCOUNT_DELETION'))
$$;
CREATE FUNCTION org_settings_clear_member(v JSONB,member TEXT) RETURNS JSONB
 LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog AS $$
DECLARE k TEXT;slot TEXT;result JSONB;
BEGIN result=v;
 FOR k IN SELECT unnest(ARRAY['review_fallback','platform_notices','data_protection','billing']) LOOP
  FOR slot IN SELECT unnest(ARRAY['primary','substitute']) LOOP
   IF result #>> ARRAY['responsibilities',k,slot]=member THEN result=jsonb_set(result,ARRAY['responsibilities',k,slot],to_jsonb(''::text),false);END IF;
  END LOOP;
 END LOOP;RETURN result;
END $$;
CREATE FUNCTION org_settings_version_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;current_revision BIGINT;current_at TIMESTAMPTZ;current_values JSONB;creator UUID;item RECORD;slot TEXT;member TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT COALESCE(public.org_settings_values_valid(NEW.settings_values),false) OR NOT COALESCE(public.org_legal_name_valid(NEW.reason,true),false) THEN RAISE EXCEPTION USING ERRCODE='P8102',MESSAGE='invalid organization settings';END IF;
 SELECT h.revision,v.effective_at,v.settings_values INTO current_revision,current_at,current_values FROM public.org_settings_heads h JOIN public.org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision) WHERE h.org_id=NEW.org_id;
 IF NEW.revision IS NULL OR NEW.revision<1 OR(current_revision IS NULL AND NEW.revision<>1) OR(current_revision IS NOT NULL AND(current_revision=9223372036854775807 OR NEW.revision<>current_revision+1)) THEN RAISE EXCEPTION USING ERRCODE='P8101',MESSAGE='stale organization settings';END IF;
 NEW.effective_at=clock_timestamp();IF current_at IS NOT NULL AND NEW.effective_at<current_at THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings clock unavailable';END IF;
 IF NEW.source IN('ONBOARDING','BOOTSTRAP') THEN
  SELECT membership_id INTO creator FROM public.org_onboarding_commands WHERE org_id=NEW.org_id AND kind='CREATED';
  IF creator IS NULL OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings initialization source unavailable';END IF;
  IF NEW.source='ONBOARDING' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_onboarding_commands WHERE org_id=NEW.org_id AND kind='CREATED' AND operation_id=NEW.operation_id AND actor_id=NEW.author_account_id AND membership_id=NEW.author_membership_id) THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings initialization source unavailable';END IF;
  ELSE
   IF NEW.author_account_id IS NOT NULL OR NEW.author_membership_id IS NOT NULL OR NEW.operation_id IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings initialization source unavailable';END IF;
   IF NOT public.org_settings_member_available(NEW.org_id,creator) THEN creator=NULL;END IF;
  END IF;
  IF NEW.settings_values<>public.org_settings_defaults(NEW.settings_values->>'time_zone',creator::text) THEN RAISE EXCEPTION USING ERRCODE='P8102',MESSAGE='unsafe initial settings';END IF;
 ELSIF NEW.source='COMMAND' THEN
  IF root_state NOT IN('PENDING_SETUP','ACTIVE') OR NEW.author_account_id IS NULL OR NEW.author_membership_id IS NULL OR NEW.operation_id IS NULL
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.author_membership_id AND account_id=NEW.author_account_id AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings author unavailable';END IF;
 ELSIF NEW.source='MEMBERSHIP_REDUCTION' THEN
  IF NEW.source_membership_id IS NULL OR NEW.source_membership_revision IS NULL OR NEW.source_membership_revision<2 OR NEW.reason<>'DESIGNATION_CLEARED'
   OR NEW.author_account_id IS NOT NULL OR NEW.author_membership_id IS NOT NULL OR NEW.operation_id IS NOT NULL
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.source_membership_id AND revision=NEW.source_membership_revision AND state IN('SUSPENDED','ENDED'))
   OR NEW.settings_values IS DISTINCT FROM public.org_settings_clear_member(current_values,NEW.source_membership_id::text) OR NEW.settings_values=current_values THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings reduction source unavailable';END IF;
 ELSE RAISE EXCEPTION USING ERRCODE='P8102',MESSAGE='invalid settings source';END IF;
 IF NEW.source<>'MEMBERSHIP_REDUCTION' THEN
  FOR item IN SELECT * FROM jsonb_each(NEW.settings_values->'responsibilities') LOOP
   FOR slot IN SELECT unnest(ARRAY['primary','substitute']) LOOP
    member=item.value->>slot;
    IF member<>'' THEN
     IF NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=member::uuid) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
     IF NOT public.org_settings_member_available(NEW.org_id,member::uuid) THEN RAISE EXCEPTION USING ERRCODE='P8103',MESSAGE='settings member unavailable',HINT='responsibilities.'||item.key||'.'||slot;END IF;
    END IF;
   END LOOP;
  END LOOP;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_settings_version_guard BEFORE INSERT ON org_settings_versions FOR EACH ROW EXECUTE FUNCTION org_settings_version_guard();
CREATE FUNCTION org_settings_designation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE declared TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT settings_values #>> ARRAY['responsibilities',NEW.responsibility,NEW.slot] INTO declared FROM public.org_settings_versions WHERE org_id=NEW.org_id AND revision=NEW.settings_revision;
 IF declared IS NULL OR declared='' OR declared IS DISTINCT FROM NEW.member_id::text THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings designation projection unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_settings_designation_guard BEFORE INSERT ON org_settings_designations FOR EACH ROW EXECUTE FUNCTION org_settings_designation_guard();
CREATE FUNCTION org_settings_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR NOT EXISTS(SELECT 1 FROM public.org_settings_versions WHERE org_id=NEW.org_id AND revision=NEW.settings_revision AND source='MEMBERSHIP_REDUCTION') THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_settings_notice_guard BEFORE INSERT ON org_settings_notices FOR EACH ROW EXECUTE FUNCTION org_settings_notice_guard();
CREATE FUNCTION org_settings_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE item RECORD;slot TEXT;member TEXT;
BEGIN
 FOR item IN SELECT * FROM jsonb_each(NEW.settings_values->'responsibilities') LOOP
  FOR slot IN SELECT unnest(ARRAY['primary','substitute']) LOOP
   member=item.value->>slot;IF member<>'' THEN INSERT INTO public.org_settings_designations(org_id,settings_revision,responsibility,slot,member_id) VALUES(NEW.org_id,NEW.revision,item.key,slot,member::uuid);END IF;
  END LOOP;
 END LOOP;
 IF NEW.source='MEMBERSHIP_REDUCTION' THEN INSERT INTO public.org_settings_notices(org_id,settings_revision) VALUES(NEW.org_id,NEW.revision);END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_settings_project AFTER INSERT ON org_settings_versions FOR EACH ROW EXECUTE FUNCTION org_settings_project();
CREATE FUNCTION org_settings_head_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.revision IS NULL OR(TG_OP='INSERT' AND NEW.revision<>1) OR(TG_OP='UPDATE' AND(NEW.org_id<>OLD.org_id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1)) THEN RAISE EXCEPTION USING ERRCODE='P8101',MESSAGE='stale organization settings';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_settings_head_guard BEFORE INSERT OR UPDATE ON org_settings_heads FOR EACH ROW EXECUTE FUNCTION org_settings_head_guard();
CREATE FUNCTION org_settings_root_zone_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_zone TEXT;
BEGIN
 IF NEW.time_zone IS NOT DISTINCT FROM OLD.time_zone THEN RETURN NEW;END IF;
 SELECT v.settings_values->>'time_zone' INTO selected_zone FROM public.org_settings_heads h JOIN public.org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision) WHERE h.org_id=NEW.org_id;
 IF selected_zone IS NOT NULL AND NEW.time_zone IS DISTINCT FROM selected_zone THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='versioned settings own the organization zone';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_settings_root_zone_guard BEFORE UPDATE OF time_zone ON organizations FOR EACH ROW EXECUTE FUNCTION org_settings_root_zone_guard();
CREATE FUNCTION org_settings_publish_zone() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE zone TEXT;
BEGIN
 SELECT settings_values->>'time_zone' INTO zone FROM public.org_settings_versions WHERE org_id=NEW.org_id AND revision=NEW.revision;
 UPDATE public.organizations SET time_zone=zone,revision=revision+1 WHERE org_id=NEW.org_id AND time_zone IS DISTINCT FROM zone;RETURN NEW;
END $$;
CREATE TRIGGER org_settings_publish_zone AFTER INSERT OR UPDATE ON org_settings_heads FOR EACH ROW EXECUTE FUNCTION org_settings_publish_zone();
-- Deferred checks also run after the existing account-erasure definer restores
-- its GUC. The insert guard already proved NEW's scope. Reinstall that actual
-- event scope only inside this invoker check, then restore it; no definer/bypass.
CREATE FUNCTION org_settings_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_scope TEXT;published BOOLEAN;
BEGIN
 prior_scope=current_setting('anpfuel.org_id',true);
 PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT EXISTS(SELECT 1 FROM public.org_settings_heads WHERE org_id=NEW.org_id AND revision>=NEW.revision) INTO published;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_scope,''),true);
 IF NOT published THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings publication unavailable';END IF;RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER org_settings_published AFTER INSERT ON org_settings_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_settings_published();

CREATE FUNCTION org_settings_initialize_onboarding() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE zone TEXT;
BEGIN
 IF NEW.kind<>'CREATED' THEN RETURN NEW;END IF;
 SELECT time_zone INTO zone FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 INSERT INTO public.org_settings_versions(org_id,revision,settings_values,source,author_account_id,author_membership_id,reason,operation_id)
 VALUES(NEW.org_id,1,public.org_settings_defaults(zone,NEW.membership_id::text),'ONBOARDING',NEW.actor_id,NEW.membership_id,'Organization creation',NEW.operation_id);
 INSERT INTO public.org_settings_heads(org_id,revision) VALUES(NEW.org_id,1);RETURN NEW;
END $$;
CREATE TRIGGER org_settings_initialize_onboarding AFTER INSERT ON org_onboarding_commands FOR EACH ROW EXECUTE FUNCTION org_settings_initialize_onboarding();
CREATE FUNCTION org_settings_bootstrap(selected_org UUID) RETURNS BIGINT LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE current_revision BIGINT;zone TEXT;creator UUID;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT time_zone INTO zone FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT revision INTO current_revision FROM public.org_settings_heads WHERE org_id=selected_org;IF current_revision IS NOT NULL THEN RETURN current_revision;END IF;
 SELECT membership_id INTO creator FROM public.org_onboarding_commands WHERE org_id=selected_org AND kind='CREATED';
 IF creator IS NULL THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings initialization source unavailable';END IF;
 IF NOT public.org_settings_member_available(selected_org,creator) THEN creator=NULL;END IF;
 INSERT INTO public.org_settings_versions(org_id,revision,settings_values,source,reason) VALUES(selected_org,1,public.org_settings_defaults(zone,creator::text),'BOOTSTRAP','Legacy settings initialization');
 INSERT INTO public.org_settings_heads(org_id,revision) VALUES(selected_org,1);RETURN 1;
END $$;
CREATE FUNCTION org_settings_consume_membership() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE current_revision BIGINT;current_values JSONB;next_values JSONB;
BEGIN
 SELECT h.revision,v.settings_values INTO current_revision,current_values FROM public.org_settings_heads h JOIN public.org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision)
 WHERE h.org_id=NEW.org_id AND EXISTS(SELECT 1 FROM public.org_settings_designations d WHERE d.org_id=h.org_id AND d.settings_revision=h.revision AND d.member_id=NEW.id);
 IF NOT FOUND THEN RETURN NEW;END IF;
 IF current_revision=9223372036854775807 THEN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='settings revision unavailable';END IF;
 next_values=public.org_settings_clear_member(current_values,NEW.id::text);
 INSERT INTO public.org_settings_versions(org_id,revision,settings_values,source,reason,source_membership_id,source_membership_revision)
 VALUES(NEW.org_id,current_revision+1,next_values,'MEMBERSHIP_REDUCTION','DESIGNATION_CLEARED',NEW.id,NEW.revision);
 UPDATE public.org_settings_heads SET revision=current_revision+1 WHERE org_id=NEW.org_id;RETURN NEW;
END $$;
CREATE TRIGGER org_settings_consume_membership AFTER UPDATE OF state ON org_memberships FOR EACH ROW WHEN(NEW.state IN('SUSPENDED','ENDED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_settings_consume_membership();
DO $$ DECLARE owner_name TEXT;BEGIN
 SELECT pg_catalog.pg_get_userbyid(relowner) INTO owner_name FROM pg_catalog.pg_class WHERE oid='public.org_memberships'::regclass;
 EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON public.org_settings_versions,public.org_settings_designations,public.org_settings_notices TO %I',owner_name);
 EXECUTE pg_catalog.format('GRANT SELECT,INSERT,UPDATE ON public.org_settings_heads TO %I',owner_name);
END $$;
