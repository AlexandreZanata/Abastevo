-- EMP-IDN-002: descriptive organization records do not provision access or
-- change identity lifecycle revisions. Existing identities remain profile-pending.
CREATE FUNCTION org_employee_text_valid(value TEXT,max_bytes INTEGER,required BOOLEAN) RETURNS BOOLEAN
 LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT value IS NOT NULL AND octet_length(value)<=max_bytes AND value=btrim(value)
  AND (NOT required OR value<>'') AND value !~ '[[:cntrl:]]'
  AND value !~ U&'[\00a0\1680\2000-\200a\2028\2029\202f\205f\3000\202a-\202e\2066-\2069]'
$$;
CREATE TABLE org_employee_profiles(
 org_id UUID NOT NULL,employee_id UUID NOT NULL,
 display_name TEXT NOT NULL,registration_number TEXT NOT NULL DEFAULT '',
 work_email TEXT NOT NULL DEFAULT '',work_phone TEXT NOT NULL DEFAULT '',job_title TEXT NOT NULL DEFAULT '',
 home_unit_id UUID,revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,employee_id),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_employee_profile_unit_fkey FOREIGN KEY(org_id,home_unit_id) REFERENCES org_units(org_id,id),
 CHECK(org_employee_text_valid(display_name,200,true)),
 CHECK(org_employee_text_valid(registration_number,64,false)),
 CHECK(org_employee_text_valid(job_title,120,false)),
 CHECK(work_email='' OR (octet_length(work_email)<=254 AND work_email=lower(work_email)
  AND work_email ~ '^[!-?A-~]+@[!-?A-~]+$')),
 CHECK(work_phone='' OR work_phone ~ '^\+[1-9][0-9]{7,14}$')
);
CREATE UNIQUE INDEX org_employee_registration_unique ON org_employee_profiles(org_id,lower(registration_number)) WHERE registration_number<>'';
CREATE INDEX org_employee_match_name ON org_employee_profiles(org_id,lower(regexp_replace(display_name,' +',' ','g')));
CREATE INDEX org_employee_home_unit ON org_employee_profiles(org_id,home_unit_id) WHERE home_unit_id IS NOT NULL;
ALTER TABLE org_employee_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_employee_profiles FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_profile_scope ON org_employee_profiles
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_employee_profiles FROM PUBLIC;
CREATE TRIGGER org_employee_profile_history BEFORE DELETE OR TRUNCATE ON org_employee_profiles
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_employee_profile_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept employee changes';
 END IF;
 IF NOT COALESCE(public.org_employee_text_valid(NEW.display_name,200,true),false)
  OR NOT COALESCE(public.org_employee_text_valid(NEW.registration_number,64,false),false)
  OR NOT COALESCE(public.org_employee_text_valid(NEW.job_title,120,false),false)
  OR NEW.work_email IS NULL OR (NEW.work_email<>'' AND (octet_length(NEW.work_email)>254
   OR NEW.work_email<>lower(NEW.work_email) OR NEW.work_email !~ '^[!-?A-~]+@[!-?A-~]+$'))
  OR NEW.work_phone IS NULL OR (NEW.work_phone<>'' AND NEW.work_phone !~ '^\+[1-9][0-9]{7,14}$')
  OR NEW.revision IS NULL OR NEW.revision<1 THEN
  RAISE EXCEPTION USING ERRCODE='P7902',MESSAGE='invalid private employee profile';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id AND merged_into IS NULL) THEN
  -- A consumed unit archive may clear a retained merged record's current home
  -- link too. This exception changes no descriptive field or identity state.
  IF TG_OP<>'UPDATE' THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
  IF NEW.home_unit_id IS NOT NULL OR OLD.home_unit_id IS NULL
   OR (NEW.display_name,NEW.registration_number,NEW.work_email,NEW.work_phone,NEW.job_title)
    IS DISTINCT FROM (OLD.display_name,OLD.registration_number,OLD.work_email,OLD.work_phone,OLD.job_title)
   OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=OLD.home_unit_id AND state='ARCHIVED') THEN
   RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';
  END IF;
 END IF;
 IF NEW.registration_number<>'' AND EXISTS(SELECT 1 FROM public.org_employee_profiles
  WHERE org_id=NEW.org_id AND employee_id<>NEW.employee_id AND lower(registration_number)=lower(NEW.registration_number)) THEN
  RAISE EXCEPTION USING ERRCODE='P7901',MESSAGE='duplicate employee registration';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial employee profile'; END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.employee_id<>OLD.employee_id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid employee profile revision or identity';
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF NEW.home_unit_id IS NOT NULL AND (TG_OP='INSERT' OR NEW.home_unit_id IS DISTINCT FROM OLD.home_unit_id) THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.home_unit_id AND state='ACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_employee_profile_revision BEFORE INSERT OR UPDATE ON org_employee_profiles
 FOR EACH ROW EXECUTE FUNCTION org_employee_profile_guard();

CREATE TABLE org_employee_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES accounts(id),employee_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('CREATE','COMPLETE','CHANGE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 result_profile_revision BIGINT NOT NULL CHECK(result_profile_revision>0),
 result_identity_revision BIGINT NOT NULL CHECK(result_identity_revision>0),
 result_state TEXT NOT NULL CHECK(result_state IN ('ACTIVE','INACTIVE','TERMINATED')),
 duplicate_reviewed BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_profiles(org_id,employee_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK(kind<>'CREATE' OR (result_profile_revision=1 AND result_identity_revision=1 AND result_state='ACTIVE')),
 CHECK(kind<>'COMPLETE' OR result_profile_revision=1),CHECK(kind<>'CHANGE' OR result_profile_revision>1)
);
ALTER TABLE org_employee_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_employee_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_command_scope ON org_employee_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_employee_commands FROM PUBLIC;
CREATE TRIGGER org_employee_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_employee_commands
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_employee_command_time BEFORE INSERT ON org_employee_commands
 FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

-- Active employees count even without a membership. Retained historical
-- references do not count, and every unregistered FK consumer still fails closed.
CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN
  RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
  AND NOT ((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
   OR (c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
   OR (c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';
 END IF;
 SELECT count(*) INTO employee_count FROM public.org_employee_profiles p
 JOIN public.org_employee_identities e ON (e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE';
 RETURN jsonb_build_object('assets',0,'employees',employee_count,'budgets',0,'operations',0);
END $$;

-- Causal evidence for a consumed unit-archive event. The deferred composite FK
-- requires that very audited archive command and exact unit revision to commit.
ALTER TABLE org_unit_commands ADD CONSTRAINT org_unit_command_causal_unique UNIQUE(org_id,operation_id,unit_id,result_revision,kind);
CREATE TABLE org_employee_unit_clearings(
 org_id UUID NOT NULL,employee_id UUID NOT NULL,profile_revision BIGINT NOT NULL CHECK(profile_revision>1),
 operation_id UUID NOT NULL,unit_id UUID NOT NULL,unit_revision BIGINT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'ARCHIVE' CHECK(kind='ARCHIVE'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,employee_id,profile_revision),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_profiles(org_id,employee_id),
 FOREIGN KEY(org_id,operation_id,unit_id,unit_revision,kind)
  REFERENCES org_unit_commands(org_id,operation_id,unit_id,result_revision,kind) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_employee_unit_clearings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_employee_unit_clearings FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_clearing_scope ON org_employee_unit_clearings
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_employee_unit_clearings FROM PUBLIC;
CREATE TRIGGER org_employee_clearing_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_employee_unit_clearings
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_employee_clearing_time BEFORE INSERT ON org_employee_unit_clearings
 FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();
CREATE FUNCTION org_employee_consume_unit_archive() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE causal_ops UUID[];causal_op UUID;
BEGIN
 IF OLD.state<>'ACTIVE' OR NEW.state<>'ARCHIVED' OR NOT EXISTS(
  SELECT 1 FROM public.org_employee_profiles WHERE org_id=NEW.org_id AND home_unit_id=NEW.id) THEN RETURN NEW; END IF;
 SELECT array_agg(a.operation_id) INTO causal_ops FROM public.org_audit_events a
 WHERE a.org_id=NEW.org_id AND a.action='org.unit.archived' AND a.target_type='unit' AND a.target_id=NEW.id
 AND NOT EXISTS(SELECT 1 FROM public.org_unit_commands c WHERE c.org_id=a.org_id AND c.operation_id=a.operation_id);
 IF COALESCE(cardinality(causal_ops),0)<>1 THEN
  RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='employee unit archive cause unavailable';
 END IF;
 causal_op=causal_ops[1];
 WITH cleared AS (
 UPDATE public.org_employee_profiles SET home_unit_id=NULL,revision=revision+1
 WHERE org_id=NEW.org_id AND home_unit_id=NEW.id RETURNING org_id,employee_id,revision)
 INSERT INTO public.org_employee_unit_clearings(org_id,employee_id,profile_revision,operation_id,unit_id,unit_revision)
 SELECT org_id,employee_id,revision,causal_op,NEW.id,NEW.revision FROM cleared;
 RETURN NEW;
END $$;
CREATE TRIGGER org_employee_unit_archive AFTER UPDATE OF state ON org_units
 FOR EACH ROW EXECUTE FUNCTION org_employee_consume_unit_archive();
