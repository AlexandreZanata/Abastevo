-- EMP-ORG-006: current cost-centre metadata and immutable code reservations.
-- Every old normalized code retains the same stable scoped centre identity.
CREATE TABLE org_cost_centres(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL,
 name TEXT NOT NULL,code TEXT NOT NULL,
 code_key TEXT GENERATED ALWAYS AS(lower(code)) STORED,
 state TEXT NOT NULL CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid)
);
CREATE TABLE org_cost_centre_codes(
 org_id UUID NOT NULL,code TEXT NOT NULL,
 code_key TEXT GENERATED ALWAYS AS(lower(code)) STORED,
 centre_id UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,code_key),UNIQUE(org_id,code_key,centre_id),
 CONSTRAINT org_cost_centre_codes_centre_fkey FOREIGN KEY(org_id,centre_id) REFERENCES org_cost_centres(org_id,id) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_cost_centres ADD CONSTRAINT org_cost_centre_current_code_fkey
 FOREIGN KEY(org_id,code_key,id) REFERENCES org_cost_centre_codes(org_id,code_key,centre_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE org_cost_centre_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES accounts(id),centre_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('CREATE','CHANGE','ARCHIVE','RESTORE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 result_state TEXT NOT NULL CHECK(result_state IN('ACTIVE','ARCHIVED')),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CONSTRAINT org_cost_centre_commands_centre_fkey FOREIGN KEY(org_id,centre_id) REFERENCES org_cost_centres(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((kind='CREATE' AND result_state='ACTIVE' AND result_revision=1)
  OR (kind IN('CHANGE','RESTORE') AND result_revision>1 AND (kind<>'RESTORE' OR result_state='ACTIVE'))
  OR (kind='ARCHIVE' AND result_revision>1 AND result_state='ARCHIVED'))
);
ALTER TABLE org_cost_centres ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_cost_centres FORCE ROW LEVEL SECURITY;
CREATE POLICY org_cost_centre_scope ON org_cost_centres
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_cost_centre_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_cost_centre_codes FORCE ROW LEVEL SECURITY;
CREATE POLICY org_cost_centre_code_scope ON org_cost_centre_codes
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_cost_centre_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_cost_centre_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_cost_centre_command_scope ON org_cost_centre_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_cost_centres,org_cost_centre_codes,org_cost_centre_commands FROM PUBLIC;
CREATE TRIGGER org_cost_centre_history BEFORE DELETE OR TRUNCATE ON org_cost_centres FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_cost_centre_code_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_cost_centre_codes FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_cost_centre_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_cost_centre_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_cost_centre_command_time BEFORE INSERT ON org_cost_centre_commands FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

-- Owning consumers extend this exact source function with actual scoped counts
-- and the root-serialized admission contract. No current operational consumer
-- exists. An unknown FK to either a centre or a reserved alias fails closed.
CREATE FUNCTION org_cost_centre_current_usage(selected_org UUID,selected_centre UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass)
  AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey')
   OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey')
   OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',0,'employees',0,'budgets',0,'operations',0);
END $$;
CREATE FUNCTION org_cost_centre_admit(selected_org UUID,selected_centre UUID) RETURNS void
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept cost-centre work';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
END $$;

CREATE FUNCTION org_cost_centre_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;reserved UUID;usage JSONB;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT root_ready OR root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept cost-centre changes';END IF;
 IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid
  OR NOT public.org_legal_name_valid(NEW.name,true) OR NOT public.org_legal_name_valid(NEW.code,true)
  OR NEW.state IS NULL OR NEW.state NOT IN('ACTIVE','ARCHIVED') OR NEW.revision IS NULL OR NEW.revision<1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid cost-centre profile';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ACTIVE' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial cost centre';END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid cost-centre identity or revision';END IF;
  IF OLD.state='ACTIVE' AND NEW.state='ARCHIVED' THEN
   usage=public.org_cost_centre_current_usage(OLD.org_id,OLD.id);
   IF usage IS NULL OR jsonb_typeof(usage)<>'object' THEN RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';END IF;
   IF NOT usage ?& ARRAY['assets','employees','budgets','operations'] OR (SELECT count(*) FROM jsonb_object_keys(usage))<>4
    OR jsonb_typeof(usage->'assets')<>'number' OR jsonb_typeof(usage->'employees')<>'number' OR jsonb_typeof(usage->'budgets')<>'number' OR jsonb_typeof(usage->'operations')<>'number'
    OR usage->>'assets' !~ '^[0-9]+$' OR usage->>'employees' !~ '^[0-9]+$' OR usage->>'budgets' !~ '^[0-9]+$' OR usage->>'operations' !~ '^[0-9]+$' THEN RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';END IF;
   IF (usage->>'assets')::bigint>0 OR (usage->>'employees')::bigint>0 OR (usage->>'budgets')::bigint>0 OR (usage->>'operations')::bigint>0 THEN RAISE EXCEPTION USING ERRCODE='P7803',MESSAGE='structure currently in use';END IF;
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 SELECT centre_id INTO reserved FROM public.org_cost_centre_codes WHERE org_id=NEW.org_id AND code_key=lower(NEW.code);
 IF FOUND AND reserved<>NEW.id THEN RAISE EXCEPTION USING ERRCODE='P7801',MESSAGE='duplicate cost-centre code';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_cost_centre_revision BEFORE INSERT OR UPDATE ON org_cost_centres FOR EACH ROW EXECUTE FUNCTION org_cost_centre_guard();
CREATE FUNCTION org_cost_centre_code_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state IN('PENDING_SETUP','ACTIVE') FOR UPDATE;
 IF NOT FOUND OR NOT public.org_legal_name_valid(NEW.code,true) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid cost-centre reservation';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=NEW.org_id AND id=NEW.centre_id AND lower(code)=lower(NEW.code)) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private current reference not found';END IF;
 NEW.created_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_cost_centre_code_reservation BEFORE INSERT ON org_cost_centre_codes FOR EACH ROW EXECUTE FUNCTION org_cost_centre_code_guard();
CREATE FUNCTION org_cost_centre_reserve_code() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 INSERT INTO public.org_cost_centre_codes(org_id,code,centre_id) VALUES(NEW.org_id,NEW.code,NEW.id) ON CONFLICT(org_id,code_key) DO NOTHING;
 RETURN NULL;
END $$;
CREATE TRIGGER org_cost_centre_code_capture AFTER INSERT OR UPDATE ON org_cost_centres FOR EACH ROW EXECUTE FUNCTION org_cost_centre_reserve_code();
