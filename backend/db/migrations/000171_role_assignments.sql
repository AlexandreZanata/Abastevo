-- EMP-AUTH-002. Fixed-role periods are storage candidates, never actor proof.
CREATE TABLE org_role_assignments(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 membership_id UUID NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('OWNER','ADMIN','FLEET_MANAGER','SUPERVISOR','FINANCE','APPROVER','DRIVER','OPERATIONS_STAFF','AUDITOR','STATION_ADMIN','STATION_MANAGER','STATION_ATTENDANT','DATA_MANAGER')),
 scope_type TEXT NOT NULL CHECK(scope_type IN ('ORG','SELF')),
 scope_target UUID NOT NULL,
 valid_from TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_from)),
 valid_to TIMESTAMPTZ CHECK(isfinite(valid_to) AND valid_to>valid_from),
 assigned_by UUID NOT NULL,
 reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 280 AND reason !~ '[[:cntrl:]]'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 ended_at TIMESTAMPTZ CHECK(isfinite(ended_at)),
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,assigned_by) REFERENCES org_memberships(org_id,id),
 CHECK((scope_type='ORG' AND scope_target=org_id) OR (scope_type='SELF' AND scope_target=membership_id)),
 CHECK(role<>'OWNER' OR scope_type='ORG'),
 -- STATION authority validation belongs to the later scoped role task.
 CHECK(role NOT IN ('STATION_ADMIN','STATION_MANAGER','STATION_ATTENDANT')),
 CHECK((revision=1)=(ended_at IS NULL))
);
CREATE INDEX org_role_assignments_member ON org_role_assignments(org_id,membership_id,role,scope_type,scope_target);
REVOKE ALL ON org_role_assignments FROM PUBLIC;
ALTER TABLE org_role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_role_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY org_role_assignments_scope ON org_role_assignments
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_role_assignment_history BEFORE DELETE OR TRUNCATE ON org_role_assignments
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

-- Called only under the shared organization row lock. Multirange union proves
-- continuous future coverage, including adjoining finite/scheduled periods.
-- Empty cancelled intervals disappear; account-exit reservations never count.
CREATE FUNCTION org_access_owner_coverage(org UUID,exclude_member UUID,exclude_assignment UUID,candidate UUID) RETURNS BOOLEAN
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT COALESCE((SELECT range_agg(tstzrange(r.valid_from,
   CASE WHEN r.ended_at IS NOT NULL THEN greatest(r.valid_from,least(r.ended_at,r.valid_to)) ELSE r.valid_to END,'[)'))
  FROM public.org_role_assignments r
  JOIN public.org_memberships m ON (m.org_id,m.id)=(r.org_id,r.membership_id)
  JOIN public.org_employee_identities e ON (e.org_id,e.id)=(m.org_id,m.employee_id) AND e.state='ACTIVE'
  JOIN public.accounts a ON a.id=m.account_id AND a.status='active'
  WHERE r.org_id=org AND org=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid
   AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=org
   AND (m.state='ACTIVE' OR m.id=candidate) AND m.access_model='EXISTING_ACCOUNT'
   AND m.id IS DISTINCT FROM exclude_member AND r.id IS DISTINCT FROM exclude_assignment
   AND NOT EXISTS(SELECT 1 FROM public.payment_billing_exits x WHERE x.account_id=m.account_id AND x.reason='ACCOUNT_DELETION'))
  @> tstzrange(clock_timestamp(),NULL,'[)'),false)
 $$;

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
  IF EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=org AND membership_id=member AND role='OWNER')
   AND NOT public.org_access_owner_coverage(org,member,NULL,NULL) THEN RETURN 'LAST_OWNER'; END IF;
  RETURN 'ALLOW';
 END IF;
 RETURN 'UNAVAILABLE';
END $$;

CREATE FUNCTION org_role_assignment_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE account UUID;account_status TEXT;member_state TEXT;model TEXT;root_state TEXT;root_ready BOOLEAN;employee_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT account_id INTO account FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 -- Account first, then root, matching admission/erasure lock order. A bound
 -- account cannot change to another account during a membership period.
 IF account IS NOT NULL THEN SELECT status INTO account_status FROM public.accounts WHERE id=account FOR SHARE; END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 SELECT m.state,m.access_model,e.state INTO member_state,model,employee_state FROM public.org_memberships m
 JOIN public.org_employee_identities e ON (e.org_id,e.id)=(m.org_id,m.employee_id)
 WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id;
 IF TG_OP='INSERT' THEN
  IF NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE') OR account IS NULL OR account_status<>'active'
   OR member_state NOT IN ('INVITED','ACTIVE') OR employee_state<>'ACTIVE'
   OR EXISTS(SELECT 1 FROM public.payment_billing_exits WHERE account_id=account AND reason='ACCOUNT_DELETION')
   OR (NEW.role='OWNER' AND model<>'EXISTING_ACCOUNT') OR NEW.revision<>1 OR NEW.ended_at IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='membership not accepting assignment';
  END IF;
  NEW.created_at=clock_timestamp();
  IF EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=NEW.org_id AND r.id<>NEW.id
   AND (r.membership_id,r.role,r.scope_type,r.scope_target)=(NEW.membership_id,NEW.role,NEW.scope_type,NEW.scope_target)
   AND tstzrange(r.valid_from,CASE WHEN r.ended_at IS NOT NULL THEN greatest(r.valid_from,least(r.ended_at,r.valid_to)) ELSE r.valid_to END,'[)')
    && tstzrange(NEW.valid_from,NEW.valid_to,'[)')) THEN
   RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='assignment interval overlaps';
  END IF;
 ELSE
  IF OLD.ended_at IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='assignment period ended'; END IF;
  IF (NEW.org_id,NEW.id,NEW.membership_id,NEW.role,NEW.scope_type,NEW.scope_target,NEW.valid_from,NEW.assigned_by,NEW.reason)
    IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.membership_id,OLD.role,OLD.scope_type,OLD.scope_target,OLD.valid_from,OLD.assigned_by,OLD.reason)
   OR NEW.valid_to IS DISTINCT FROM OLD.valid_to OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 OR NEW.ended_at IS NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment history cannot be rewritten';
  END IF;
  NEW.created_at=OLD.created_at;NEW.ended_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_role_assignment_revision BEFORE INSERT OR UPDATE ON org_role_assignments
 FOR EACH ROW EXECUTE FUNCTION org_role_assignment_guard();

CREATE FUNCTION org_role_assignment_owner_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);
 -- Deferred checks may run after a composer selected a different org. Use
 -- the row's immutable scope, then restore the composer's transaction scope.
 PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 -- INVITED setup has no authority. Once an owner is active, a gap is refused
 -- at commit, including finite expiry. The root lock prevents write skew.
 IF EXISTS(SELECT 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE')
  OR EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_role_assignments r
   ON (r.org_id,r.membership_id)=(m.org_id,m.id) WHERE m.org_id=NEW.org_id AND m.state='ACTIVE' AND r.role='OWNER') THEN
  IF NOT public.org_access_owner_coverage(NEW.org_id,NULL,NULL,NULL) THEN
   RAISE EXCEPTION USING ERRCODE='P7501',MESSAGE='enterprise last owner protected';
  END IF;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_role_owner_coverage AFTER INSERT OR UPDATE ON org_role_assignments
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.role='OWNER') EXECUTE FUNCTION org_role_assignment_owner_guard();

CREATE FUNCTION org_membership_end_assignments() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1
 WHERE org_id=NEW.org_id AND membership_id=NEW.id AND ended_at IS NULL;
 RETURN NEW;
END $$;
CREATE TRIGGER org_membership_role_end AFTER UPDATE OF state ON org_memberships
 FOR EACH ROW WHEN(NEW.state='ENDED' AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_membership_end_assignments();

-- A personal-account suspension must not bypass the same last-owner fence.
CREATE FUNCTION org_account_access_reduction() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 PERFORM public.org_membership_check_account_exit(NEW.id);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION org_account_access_reduction() FROM PUBLIC;
CREATE TRIGGER org_account_access_reduction BEFORE UPDATE OF status ON accounts
 FOR EACH ROW WHEN(OLD.status='active' AND NEW.status IS DISTINCT FROM OLD.status) EXECUTE FUNCTION org_account_access_reduction();
