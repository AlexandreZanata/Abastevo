-- EMP-ORG-003: atomic synthetic setup. Existing accounts remain personal;
-- descriptive membership models do not convert their authoritative origin.
ALTER TABLE accounts ADD COLUMN enterprise_managed_org_id UUID REFERENCES organizations(org_id)
 CHECK(enterprise_managed_org_id<>'00000000-0000-0000-0000-000000000000'::uuid);
CREATE FUNCTION org_account_managed_identity_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.enterprise_managed_org_id IS DISTINCT FROM OLD.enterprise_managed_org_id THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='account access origin is immutable';
 END IF;
 IF TG_OP='INSERT' AND NEW.enterprise_managed_org_id IS NOT NULL THEN
  IF NEW.enterprise_managed_org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid
   OR NOT EXISTS(SELECT 1 FROM public.organizations WHERE org_id=NEW.enterprise_managed_org_id AND has_snapshot AND state IN ('PENDING_SETUP','ACTIVE')) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid managed account origin';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_account_managed_identity BEFORE INSERT OR UPDATE ON accounts
 FOR EACH ROW EXECUTE FUNCTION org_account_managed_identity_guard();

CREATE FUNCTION org_membership_account_model_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE managed_org UUID;
BEGIN
 IF NEW.account_id IS NULL THEN RETURN NEW; END IF;
 SELECT enterprise_managed_org_id INTO managed_org FROM public.accounts WHERE id=NEW.account_id FOR SHARE;
 IF (managed_org IS NOT NULL AND (managed_org<>NEW.org_id OR NEW.access_model<>'MANAGED'))
  OR (NEW.state='ACTIVE' AND NEW.access_model='MANAGED' AND managed_org IS NULL) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='account origin does not match membership';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_membership_account_model BEFORE INSERT OR UPDATE ON org_memberships
 FOR EACH ROW EXECUTE FUNCTION org_membership_account_model_guard();

CREATE TABLE org_units(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120 AND name=btrim(name) AND name !~ '[[:cntrl:]]' AND name !~ U&'[\202a-\202e\2066-\2069]'),
 code TEXT NOT NULL CHECK(length(btrim(code)) BETWEEN 1 AND 120 AND code=btrim(code) AND code !~ '[[:cntrl:]]' AND code !~ U&'[\202a-\202e\2066-\2069]'),
 time_zone TEXT NOT NULL CHECK(length(time_zone) BETWEEN 1 AND 128),
 state TEXT NOT NULL CHECK(state IN ('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id)
);
CREATE UNIQUE INDEX org_units_active_name ON org_units(org_id,lower(name)) WHERE state='ACTIVE';
CREATE UNIQUE INDEX org_units_active_code ON org_units(org_id,lower(code)) WHERE state='ACTIVE';
ALTER TABLE org_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_units FORCE ROW LEVEL SECURITY;
CREATE POLICY org_units_scope ON org_units
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_units FROM PUBLIC;
CREATE TRIGGER org_units_history BEFORE DELETE OR TRUNCATE ON org_units
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_unit_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NOT root_ready OR root_state='CLOSED' THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept unit changes'; END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.time_zone) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid unit time zone';
 END IF;
 IF TG_OP='INSERT' THEN
  IF root_state NOT IN ('PENDING_SETUP','ACTIVE') OR NEW.revision<>1 OR NEW.state<>'ACTIVE' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial unit';
  END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid unit identity or revision';
  END IF;
  IF OLD.state='ACTIVE' AND NEW.state='ARCHIVED' AND NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=OLD.org_id AND id<>OLD.id AND state='ACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='P7506',MESSAGE='last active unit protected';
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_unit_revision BEFORE INSERT OR UPDATE ON org_units FOR EACH ROW EXECUTE FUNCTION org_unit_guard();

CREATE TABLE org_onboarding_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id) DEFERRABLE INITIALLY DEFERRED,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN ('CREATED','ACTIVATED')),
 actor_id UUID NOT NULL REFERENCES accounts(id),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 unit_id UUID,employee_id UUID,membership_id UUID,assignment_id UUID,
 terms_version TEXT,terms_digest TEXT,terms_kind TEXT,
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,assignment_id) REFERENCES org_role_assignments(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((kind='CREATED' AND unit_id IS NOT NULL AND employee_id IS NOT NULL AND membership_id IS NOT NULL AND assignment_id IS NOT NULL
  AND terms_version IS NOT NULL AND terms_digest IS NOT NULL AND terms_kind IS NOT NULL
  AND terms_version='enterprise-synthetic-dpa-v1' AND terms_digest='590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3'
  AND terms_kind='SYNTHETIC' AND result_revision=1)
  OR (kind='ACTIVATED' AND unit_id IS NULL AND employee_id IS NULL AND membership_id IS NULL AND assignment_id IS NULL
   AND terms_version IS NULL AND terms_digest IS NULL AND terms_kind IS NULL AND result_revision>1))
);
CREATE UNIQUE INDEX org_onboarding_account_operation ON org_onboarding_commands(actor_id,operation_id) WHERE kind='CREATED';
CREATE UNIQUE INDEX org_onboarding_creation ON org_onboarding_commands(org_id) WHERE kind='CREATED';
CREATE UNIQUE INDEX org_onboarding_activation ON org_onboarding_commands(org_id) WHERE kind='ACTIVATED';
ALTER TABLE org_onboarding_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_onboarding_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_onboarding_scope ON org_onboarding_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
-- Account-owned creation acknowledgements only; this is not current org access.
CREATE POLICY org_onboarding_account_read ON org_onboarding_commands FOR SELECT
 USING(kind='CREATED' AND actor_id=NULLIF(current_setting('anpfuel.creation_account',true),'')::uuid);
REVOKE ALL ON org_onboarding_commands FROM PUBLIC;
CREATE TRIGGER org_onboarding_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_onboarding_commands
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_onboarding_created_time() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN NEW.created_at=clock_timestamp();RETURN NEW;END $$;
CREATE TRIGGER org_onboarding_created_time BEFORE INSERT ON org_onboarding_commands FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

-- Narrow trigger-only inventory for the pending ownership ceiling. Every
-- owning table and definer must have the same ordinary migration owner.
CREATE POLICY org_membership_owner_inventory ON org_memberships FOR SELECT
 USING(current_user=pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass))
  AND account_id=NULLIF(current_setting('anpfuel.owner_account',true),'')::uuid);
-- Invoker PL/pgSQL short-circuits before private subqueries for runtime roles.
-- Inline policy subqueries would impose unrelated membership SELECT grants
-- on every ordinary root/role reader even when the owner predicate is false.
CREATE FUNCTION org_owner_role_visible(org UUID,member UUID) RETURNS BOOLEAN
 LANGUAGE plpgsql STABLE SET search_path=pg_catalog AS $$
BEGIN
 IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_role_assignments'::regclass)) THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM public.org_memberships m WHERE m.org_id=org AND m.id=member
  AND m.account_id=NULLIF(current_setting('anpfuel.owner_account',true),'')::uuid);
END $$;
CREATE FUNCTION org_owner_root_visible(org UUID) RETURNS BOOLEAN
 LANGUAGE plpgsql STABLE SET search_path=pg_catalog AS $$
BEGIN
 IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.organizations'::regclass)) THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_role_assignments r ON (r.org_id,r.membership_id)=(m.org_id,m.id)
  WHERE m.org_id=org AND m.account_id=NULLIF(current_setting('anpfuel.owner_account',true),'')::uuid AND r.role='OWNER');
END $$;
CREATE POLICY org_role_owner_inventory ON org_role_assignments FOR SELECT
 USING(role='OWNER' AND public.org_owner_role_visible(org_id,membership_id));
CREATE POLICY org_root_owner_inventory ON organizations FOR SELECT
 USING(public.org_owner_root_visible(org_id));
CREATE FUNCTION org_pending_owner_count(target UUID,except_org UUID) RETURNS BIGINT
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE prior_account TEXT;result BIGINT;coherent BOOLEAN;
BEGIN
 SELECT count(*)=4 AND bool_and(pg_get_userbyid(relowner)=current_user) INTO coherent FROM pg_class
 WHERE oid IN ('public.accounts'::regclass,'public.organizations'::regclass,'public.org_memberships'::regclass,'public.org_role_assignments'::regclass);
 IF NOT coherent THEN RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable'; END IF;
 PERFORM 1 FROM public.accounts WHERE id=target FOR UPDATE;
 prior_account=current_setting('anpfuel.owner_account',true);PERFORM set_config('anpfuel.owner_account',target::text,true);
 SELECT count(DISTINCT r.org_id) INTO result FROM public.org_role_assignments r
 JOIN public.org_memberships m ON (m.org_id,m.id)=(r.org_id,r.membership_id)
 JOIN public.organizations o ON o.org_id=r.org_id AND o.state='PENDING_SETUP'
 WHERE m.account_id=target AND m.state<>'ENDED' AND r.role='OWNER' AND r.org_id<>except_org
  AND r.ended_at IS NULL AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp());
 PERFORM set_config('anpfuel.owner_account',COALESCE(prior_account,''),true);RETURN result;
END $$;
REVOKE ALL ON FUNCTION org_pending_owner_count(UUID,UUID) FROM PUBLIC;
CREATE FUNCTION org_role_owner_account_guard() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE account UUID;managed_org UUID;root_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT account_id INTO account FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id;
 IF account IS NULL THEN RETURN NEW; END IF;
 SELECT enterprise_managed_org_id INTO managed_org FROM public.accounts WHERE id=account FOR UPDATE;
 IF managed_org IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P7505',MESSAGE='owner requires personal account'; END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id;
 IF root_state='PENDING_SETUP' AND (NEW.valid_to IS NULL OR NEW.valid_to>clock_timestamp())
  AND public.org_pending_owner_count(account,NEW.org_id)>=3 THEN
  RAISE EXCEPTION USING ERRCODE='P7503',MESSAGE='pending organization limit reached';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION org_role_owner_account_guard() FROM PUBLIC;
CREATE TRIGGER org_role_assignment_account BEFORE INSERT ON org_role_assignments
 FOR EACH ROW WHEN(NEW.role='OWNER') EXECUTE FUNCTION org_role_owner_account_guard();

CREATE FUNCTION org_setup_activation_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND state='ACTIVE')
  OR NOT EXISTS(SELECT 1 FROM public.org_onboarding_commands WHERE org_id=NEW.org_id AND kind='CREATED'
   AND terms_version='enterprise-synthetic-dpa-v1' AND terms_digest='590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3' AND terms_kind='SYNTHETIC')
  OR NOT public.org_access_owner_coverage(NEW.org_id,NULL,NULL,NULL) THEN
  RAISE EXCEPTION USING ERRCODE='P7504',MESSAGE='organization setup incomplete';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_setup_activation BEFORE UPDATE OF state ON organizations
 FOR EACH ROW WHEN(OLD.state='PENDING_SETUP' AND NEW.state='ACTIVE') EXECUTE FUNCTION org_setup_activation_guard();

CREATE FUNCTION org_onboarding_composition_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;complete BOOLEAN;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NEW.kind='CREATED' THEN
  SELECT EXISTS(SELECT 1 FROM public.organizations o
   JOIN public.org_units u ON (u.org_id,u.id)=(o.org_id,NEW.unit_id) AND u.state='ACTIVE' AND u.code='MAIN' AND u.time_zone=o.time_zone
   JOIN public.org_employee_identities e ON (e.org_id,e.id)=(o.org_id,NEW.employee_id) AND e.state='ACTIVE'
   JOIN public.org_memberships m ON (m.org_id,m.id)=(o.org_id,NEW.membership_id) AND m.employee_id=e.id AND m.account_id=NEW.actor_id AND m.access_model='EXISTING_ACCOUNT' AND m.state='ACTIVE'
   JOIN public.org_role_assignments r ON (r.org_id,r.id)=(o.org_id,NEW.assignment_id) AND r.membership_id=m.id AND r.assigned_by=m.id AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=o.org_id AND r.ended_at IS NULL
    AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp())
   WHERE o.org_id=NEW.org_id AND o.has_snapshot AND o.state IN ('PENDING_SETUP','ACTIVE')) INTO complete;
 ELSE
  SELECT EXISTS(SELECT 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE' AND revision=NEW.result_revision) INTO complete;
 END IF;
 IF NOT complete THEN RAISE EXCEPTION USING ERRCODE='P7504',MESSAGE='organization setup incomplete'; END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_onboarding_composition AFTER INSERT ON org_onboarding_commands
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_onboarding_composition_guard();
