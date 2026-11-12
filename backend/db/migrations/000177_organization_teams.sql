-- EMP-ORG-007. Teams are structure, not roles. Employee-link periods and
-- automatic source reductions preserve history and share the existing root lock.
CREATE TABLE org_teams(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL,
 name TEXT NOT NULL,unit_id UUID,state TEXT NOT NULL CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),CONSTRAINT org_team_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid)
);
CREATE UNIQUE INDEX org_team_active_name_unique ON org_teams(org_id,lower(name)) WHERE state='ACTIVE';
CREATE INDEX org_team_unit ON org_teams(org_id,unit_id) WHERE unit_id IS NOT NULL;
CREATE TABLE org_team_links(
 org_id UUID NOT NULL,id UUID NOT NULL,team_id UUID NOT NULL,employee_id UUID NOT NULL,
 created_operation_id UUID NOT NULL,started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),ended_at TIMESTAMPTZ,
 end_reason TEXT NOT NULL DEFAULT '',closed_operation_id UUID,
 source_membership_id UUID,source_membership_revision BIGINT,source_employee_revision BIGINT,
 PRIMARY KEY(org_id,id),UNIQUE(org_id,team_id,id),
 CONSTRAINT org_team_link_team_fkey FOREIGN KEY(org_id,team_id) REFERENCES org_teams(org_id,id),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,source_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,created_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,closed_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK(isfinite(started_at) AND(ended_at IS NULL OR(isfinite(ended_at) AND ended_at>=started_at))),
 CHECK((ended_at IS NULL AND end_reason='' AND closed_operation_id IS NULL AND source_membership_id IS NULL AND source_membership_revision IS NULL AND source_employee_revision IS NULL)
  OR(ended_at IS NOT NULL AND end_reason='REMOVED' AND closed_operation_id IS NOT NULL AND source_membership_id IS NULL AND source_membership_revision IS NULL AND source_employee_revision IS NULL)
  OR(ended_at IS NOT NULL AND end_reason IN('EMPLOYEE_INACTIVE','EMPLOYEE_TERMINATED') AND closed_operation_id IS NULL AND source_membership_id IS NULL AND source_membership_revision IS NULL AND source_employee_revision>1)
  OR(ended_at IS NOT NULL AND end_reason IN('MEMBERSHIP_SUSPENDED','MEMBERSHIP_ENDED') AND closed_operation_id IS NULL AND source_membership_id IS NOT NULL AND source_membership_revision>1 AND source_employee_revision IS NULL))
);
CREATE UNIQUE INDEX org_team_current_employee_unique ON org_team_links(org_id,team_id,employee_id) WHERE ended_at IS NULL;
CREATE INDEX org_team_employee_current ON org_team_links(org_id,employee_id,team_id) WHERE ended_at IS NULL;
CREATE TABLE org_team_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,actor_id UUID NOT NULL REFERENCES accounts(id),
 team_id UUID NOT NULL,link_id UUID,
 kind TEXT NOT NULL CHECK(kind IN('CREATE','CHANGE','ARCHIVE','RESTORE','ADD','REMOVE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 result_state TEXT NOT NULL CHECK(result_state IN('ACTIVE','ARCHIVED')),result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_team_command_team_fkey FOREIGN KEY(org_id,team_id) REFERENCES org_teams(org_id,id),
 FOREIGN KEY(org_id,team_id,link_id) REFERENCES org_team_links(org_id,team_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK((kind IN('ADD','REMOVE'))=(link_id IS NOT NULL)),
 CHECK(kind<>'CREATE' OR(result_state='ACTIVE' AND result_revision=1)),
 CHECK(kind='CREATE' OR result_revision>1),CHECK(kind<>'RESTORE' OR result_state='ACTIVE'),CHECK(kind<>'ARCHIVE' OR result_state='ARCHIVED')
);
ALTER TABLE org_teams ENABLE ROW LEVEL SECURITY;ALTER TABLE org_teams FORCE ROW LEVEL SECURITY;
ALTER TABLE org_team_links ENABLE ROW LEVEL SECURITY;ALTER TABLE org_team_links FORCE ROW LEVEL SECURITY;
ALTER TABLE org_team_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_team_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_team_scope ON org_teams USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_team_link_scope ON org_team_links USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_team_command_scope ON org_team_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_teams,org_team_links,org_team_commands FROM PUBLIC;
CREATE TRIGGER org_team_history BEFORE DELETE OR TRUNCATE ON org_teams FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_team_link_history BEFORE DELETE OR TRUNCATE ON org_team_links FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_team_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_team_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_team_command_time BEFORE INSERT ON org_team_commands FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

CREATE FUNCTION org_team_current_usage(selected_org UUID,selected_team UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_teams WHERE org_id=selected_org AND id=selected_team) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_teams'::regclass
  AND NOT((c.conrelid='public.org_team_links'::regclass AND c.conname='org_team_link_team_fkey')
   OR(c.conrelid='public.org_team_commands'::regclass AND c.conname='org_team_command_team_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P8004',MESSAGE='team usage source unavailable';END IF;
 SELECT count(*) INTO employee_count FROM public.org_team_links l JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND l.team_id=selected_team AND l.ended_at IS NULL AND e.state='ACTIVE';
 RETURN jsonb_build_object('assets',0,'employees',employee_count,'budgets',0,'operations',0);
END $$;
CREATE FUNCTION org_team_admit(selected_org UUID,selected_team UUID) RETURNS void
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;selected_unit UUID;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept team work';END IF;
 SELECT unit_id INTO selected_unit FROM public.org_teams WHERE org_id=selected_org AND id=selected_team AND state='ACTIVE';
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
 IF selected_unit IS NOT NULL THEN PERFORM public.org_unit_admit(selected_org,selected_unit);END IF;
END $$;
CREATE FUNCTION org_team_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;usage JSONB;metadata_only BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid OR NEW.revision IS NULL OR NEW.revision<1 OR NEW.state IS NULL OR NEW.state NOT IN('ACTIVE','ARCHIVED') OR NOT COALESCE(public.org_legal_name_valid(NEW.name,true),false)
  OR NEW.name ~ U&'^[\0085\00a0\1680\2000-\200a\2028\2029\202f\205f\3000]|[\0085\00a0\1680\2000-\200a\2028\2029\202f\205f\3000]$' THEN RAISE EXCEPTION USING ERRCODE='P8003',MESSAGE='invalid team profile';END IF;
 metadata_only=false;
 IF TG_OP='UPDATE' THEN
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid team identity or revision';END IF;
  metadata_only=(NEW.name,NEW.unit_id,NEW.state) IS NOT DISTINCT FROM(OLD.name,OLD.unit_id,OLD.state);
 END IF;
 -- Safety reductions may bump revision in any organization lifecycle state;
 -- this exception permits no metadata/state/link admission change.
 IF root_state NOT IN('PENDING_SETUP','ACTIVE') AND NOT metadata_only THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept team changes';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ACTIVE' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial team';END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
  IF OLD.state='ACTIVE' AND NEW.state='ARCHIVED' THEN
   usage=public.org_team_current_usage(NEW.org_id,NEW.id);
   IF usage IS NULL OR jsonb_typeof(usage)<>'object' OR NOT usage ?& ARRAY['assets','employees','budgets','operations'] OR(SELECT count(*) FROM jsonb_object_keys(usage))<>4
    OR jsonb_typeof(usage->'assets')<>'number' OR jsonb_typeof(usage->'employees')<>'number' OR jsonb_typeof(usage->'budgets')<>'number' OR jsonb_typeof(usage->'operations')<>'number'
    OR usage->>'assets' !~ '^[0-9]+$' OR usage->>'employees' !~ '^[0-9]+$' OR usage->>'budgets' !~ '^[0-9]+$' OR usage->>'operations' !~ '^[0-9]+$' THEN RAISE EXCEPTION USING ERRCODE='P8004',MESSAGE='team usage source unavailable';END IF;
   IF(usage->>'assets')::bigint>0 OR(usage->>'employees')::bigint>0 OR(usage->>'budgets')::bigint>0 OR(usage->>'operations')::bigint>0 THEN RAISE EXCEPTION USING ERRCODE='P8007',MESSAGE='structure currently in use';END IF;
  END IF;
 END IF;
 IF NEW.unit_id IS NOT NULL AND(TG_OP='INSERT' OR NEW.unit_id IS DISTINCT FROM OLD.unit_id OR(OLD.state='ARCHIVED' AND NEW.state='ACTIVE')) THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
 END IF;
 IF NEW.state='ACTIVE' AND EXISTS(SELECT 1 FROM public.org_teams WHERE org_id=NEW.org_id AND id<>NEW.id AND state='ACTIVE' AND lower(name)=lower(NEW.name)) THEN RAISE EXCEPTION USING ERRCODE='P8001',MESSAGE='duplicate active team name';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_team_revision BEFORE INSERT OR UPDATE ON org_teams FOR EACH ROW EXECUTE FUNCTION org_team_guard();
CREATE FUNCTION org_team_link_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;employee_state TEXT;employee_revision BIGINT;team_state TEXT;selected_unit UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT state,revision INTO employee_state,employee_revision FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT state,unit_id INTO team_state,selected_unit FROM public.org_teams WHERE org_id=NEW.org_id AND id=NEW.team_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF TG_OP='INSERT' THEN
  IF root_state NOT IN('PENDING_SETUP','ACTIVE') OR team_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='team does not accept assignments';END IF;
  IF employee_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='P8005',MESSAGE='employee does not accept assignment';END IF;
  IF selected_unit IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=selected_unit AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
  IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid OR NEW.created_operation_id IS NULL OR NEW.created_operation_id='00000000-0000-0000-0000-000000000000'::uuid OR NEW.ended_at IS NOT NULL OR NEW.end_reason IS DISTINCT FROM '' OR NEW.closed_operation_id IS NOT NULL OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL OR NEW.source_employee_revision IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P8003',MESSAGE='invalid initial team link';END IF;
  IF EXISTS(SELECT 1 FROM public.org_team_links WHERE org_id=NEW.org_id AND team_id=NEW.team_id AND employee_id=NEW.employee_id AND ended_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='P8002',MESSAGE='employee already linked to team';END IF;
  NEW.started_at=clock_timestamp();
 ELSE
  IF OLD.ended_at IS NOT NULL OR NEW.ended_at IS NULL OR(NEW.org_id,NEW.id,NEW.team_id,NEW.employee_id,NEW.created_operation_id,NEW.started_at) IS DISTINCT FROM(OLD.org_id,OLD.id,OLD.team_id,OLD.employee_id,OLD.created_operation_id,OLD.started_at) THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='team link period is retained';END IF;
  IF NEW.end_reason='REMOVED' THEN
   IF NEW.closed_operation_id IS NULL OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL OR NEW.source_employee_revision IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P8006',MESSAGE='team reduction source unavailable';END IF;
  ELSIF NEW.end_reason IN('EMPLOYEE_INACTIVE','EMPLOYEE_TERMINATED') THEN
   IF NEW.source_employee_revision IS DISTINCT FROM employee_revision OR employee_revision<2 OR NEW.closed_operation_id IS NOT NULL OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL
    OR(NEW.end_reason='EMPLOYEE_INACTIVE' AND employee_state<>'INACTIVE') OR(NEW.end_reason='EMPLOYEE_TERMINATED' AND employee_state<>'TERMINATED') THEN RAISE EXCEPTION USING ERRCODE='P8006',MESSAGE='team reduction source unavailable';END IF;
  ELSIF NEW.end_reason IN('MEMBERSHIP_SUSPENDED','MEMBERSHIP_ENDED') THEN
   IF NEW.closed_operation_id IS NOT NULL OR NEW.source_employee_revision IS NOT NULL OR NEW.source_membership_revision IS NULL OR NEW.source_membership_revision<2
    OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.source_membership_id AND employee_id=NEW.employee_id AND revision=NEW.source_membership_revision AND state=CASE NEW.end_reason WHEN 'MEMBERSHIP_SUSPENDED' THEN 'SUSPENDED' ELSE 'ENDED' END) THEN RAISE EXCEPTION USING ERRCODE='P8006',MESSAGE='team reduction source unavailable';END IF;
  ELSE RAISE EXCEPTION USING ERRCODE='P8006',MESSAGE='team reduction source unavailable';END IF;
  NEW.ended_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_team_link_revision BEFORE INSERT OR UPDATE ON org_team_links FOR EACH ROW EXECUTE FUNCTION org_team_link_guard();
CREATE FUNCTION org_team_link_bump() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN UPDATE public.org_teams SET revision=revision+1 WHERE org_id=NEW.org_id AND id=NEW.team_id;RETURN NEW;END $$;
CREATE TRIGGER org_team_link_bump AFTER INSERT OR UPDATE ON org_team_links FOR EACH ROW EXECUTE FUNCTION org_team_link_bump();
CREATE FUNCTION org_team_consume_employee() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_team_links SET ended_at=clock_timestamp(),end_reason=CASE NEW.state WHEN 'INACTIVE' THEN 'EMPLOYEE_INACTIVE' ELSE 'EMPLOYEE_TERMINATED' END,source_employee_revision=NEW.revision
 WHERE org_id=NEW.org_id AND employee_id=NEW.id AND ended_at IS NULL;RETURN NEW;
END $$;
CREATE TRIGGER org_team_consume_employee AFTER UPDATE OF state ON org_employee_identities FOR EACH ROW
 WHEN(NEW.state IN('INACTIVE','TERMINATED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_team_consume_employee();
CREATE FUNCTION org_team_consume_membership() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_team_links SET ended_at=clock_timestamp(),end_reason=CASE NEW.state WHEN 'SUSPENDED' THEN 'MEMBERSHIP_SUSPENDED' ELSE 'MEMBERSHIP_ENDED' END,source_membership_id=NEW.id,source_membership_revision=NEW.revision
 WHERE org_id=NEW.org_id AND employee_id=NEW.employee_id AND ended_at IS NULL;RETURN NEW;
END $$;
CREATE TRIGGER org_team_consume_membership AFTER UPDATE OF state ON org_memberships FOR EACH ROW
 WHEN(NEW.state IN('SUSPENDED','ENDED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_team_consume_membership();
-- The existing trigger-only account-erasure definer already selects/locks each
-- org and sets its scoped GUC. Grant only its current membership-table owner
-- the bounded projection update it now consumes; no PUBLIC/runtime/login grant.
DO $$ DECLARE owner_name TEXT;BEGIN
 SELECT pg_catalog.pg_get_userbyid(relowner) INTO owner_name FROM pg_catalog.pg_class WHERE oid='public.org_memberships'::regclass;
 EXECUTE pg_catalog.format('GRANT SELECT,UPDATE ON public.org_teams,public.org_team_links TO %I',owner_name);
END $$;

-- Current unit use is the distinct union of actual home and team assignments.
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
   OR (c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
   OR (c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';
 END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM (
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE'
 ) current_employees;
 RETURN jsonb_build_object('assets',0,'employees',employee_count,'budgets',0,'operations',0);
END $$;
