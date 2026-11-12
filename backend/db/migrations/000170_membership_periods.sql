-- EMP-IDN-003. Identity/history only: no employee register, credentials,
-- runtime grants or activation. AUTH-002 replaces the denying policy hook.
CREATE TABLE org_employee_identities(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 state TEXT NOT NULL CHECK(state IN ('ACTIVE','INACTIVE','TERMINATED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 merged_into UUID,
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,merged_into) REFERENCES org_employee_identities(org_id,id),
 CHECK(merged_into IS NULL OR (merged_into<>id AND state='TERMINATED'))
);
CREATE TABLE org_memberships(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 employee_id UUID NOT NULL,
 account_id UUID REFERENCES accounts(id),
 access_model TEXT NOT NULL CHECK(access_model IN ('EXISTING_ACCOUNT','MANAGED')),
 state TEXT NOT NULL CHECK(state IN ('INVITED','ACTIVE','SUSPENDED','ENDED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 suspension_origin TEXT NOT NULL DEFAULT '' CHECK(suspension_origin IN ('','MANAGER','EMPLOYEE_INACTIVE')),
 end_reason TEXT NOT NULL DEFAULT '' CHECK(end_reason IN ('','OFFBOARDED','LEFT','ACCOUNT_DELETED','ORGANIZATION_CLOSED','ADMISSION_ENDED')),
 account_tombstone UUID CHECK(account_tombstone<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 started_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 tombstoned_at TIMESTAMPTZ,
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CHECK(account_id IS NULL OR account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK((state='ENDED')=(ended_at IS NOT NULL)),
 CHECK((account_tombstone IS NULL)=(tombstoned_at IS NULL)),
 CHECK(account_tombstone IS NULL OR (account_id IS NULL AND state='ENDED')),
 CHECK(state NOT IN ('ACTIVE','SUSPENDED') OR (account_id IS NOT NULL AND started_at IS NOT NULL)),
 CHECK(state<>'INVITED' OR (started_at IS NULL AND suspension_origin='' AND end_reason='')),
 CHECK(state<>'ACTIVE' OR (suspension_origin='' AND end_reason='')),
 CHECK(state<>'SUSPENDED' OR (suspension_origin<>'' AND end_reason='')),
 CHECK(state<>'ENDED' OR end_reason<>''),
 CHECK(started_at IS NULL OR started_at>=created_at),
 CHECK(ended_at IS NULL OR ended_at>=COALESCE(started_at,created_at))
);
CREATE UNIQUE INDEX org_memberships_live_employee ON org_memberships(org_id,employee_id) WHERE state<>'ENDED';
CREATE UNIQUE INDEX org_memberships_live_account ON org_memberships(org_id,account_id) WHERE state<>'ENDED' AND account_id IS NOT NULL;
CREATE INDEX org_memberships_account_links ON org_memberships(account_id,org_id,id) WHERE account_id IS NOT NULL;
CREATE INDEX org_employee_merge_target ON org_employee_identities(org_id,merged_into) WHERE merged_into IS NOT NULL;
REVOKE ALL ON org_employee_identities,org_memberships FROM PUBLIC;
ALTER TABLE org_employee_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_employee_identities FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_identities_scope ON org_employee_identities
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY org_memberships_scope ON org_memberships
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
-- Only the table-owning, trigger-only definer may traverse this account's
-- links without an org selection. Runtime GUC spoofing cannot satisfy owner.
CREATE POLICY org_memberships_erasure_read ON org_memberships FOR SELECT
 USING(current_user=pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass))
  AND account_id=NULLIF(current_setting('anpfuel.erasure_account',true),'')::uuid);

-- SQL policy port, not authority. AUTH-002 must derive ALLOW/LAST_OWNER from
-- real assignments under the same root lock and exclude account-deletion
-- billing-exit reservations when counting surviving owners. Unknown denies.
CREATE FUNCTION org_membership_access_policy(org UUID,member UUID,action TEXT) RETURNS TEXT
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$ SELECT 'UNAVAILABLE'::text $$;

CREATE FUNCTION org_membership_require_policy(org UUID,member UUID,action TEXT) RETURNS VOID
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE decision TEXT;
BEGIN
 decision=public.org_membership_access_policy(org,member,action);
 IF decision='LAST_OWNER' THEN
  RAISE EXCEPTION USING ERRCODE='P7501',MESSAGE='enterprise last owner protected';
 ELSIF decision IS DISTINCT FROM 'ALLOW' THEN
  RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable';
 END IF;
END $$;

CREATE FUNCTION org_identity_keep_history() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='private identity history is retained'; END $$;
CREATE TRIGGER org_employee_history BEFORE DELETE OR TRUNCATE ON org_employee_identities
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_membership_history BEFORE DELETE OR TRUNCATE ON org_memberships
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_membership_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE account_status TEXT;employee_state TEXT;erasing BOOLEAN;root_state TEXT;root_ready BOOLEAN;
BEGIN
 erasing=TG_OP='UPDATE' AND current_user=pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass))
  AND OLD.account_id=NULLIF(current_setting('anpfuel.erasure_account',true),'')::uuid;
 IF erasing THEN
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR NEW.employee_id<>OLD.employee_id OR NEW.access_model<>OLD.access_model
   OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1
   OR NEW.account_id IS NOT NULL OR NEW.account_tombstone IS NULL OR NEW.state<>'ENDED'
   OR NEW.suspension_origin<>OLD.suspension_origin
   OR NEW.created_at<>OLD.created_at OR NEW.started_at IS DISTINCT FROM OLD.started_at
   OR (OLD.state='ENDED' AND (NEW.end_reason<>OLD.end_reason OR NEW.ended_at IS DISTINCT FROM OLD.ended_at))
   OR (OLD.state<>'ENDED' AND NEW.end_reason<>'ACCOUNT_DELETED') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid account unlink';
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 -- SHARE conflicts with account status updates, unlike KEY SHARE.
 IF NEW.account_id IS NOT NULL THEN
  SELECT status INTO account_status FROM public.accounts WHERE id=NEW.account_id FOR SHARE;
  IF account_status IS NULL OR account_status='deleted'
   OR ((TG_OP='INSERT' OR NEW.state IN ('INVITED','ACTIVE')) AND
    (account_status<>'active' OR EXISTS(SELECT 1 FROM public.payment_billing_exits WHERE account_id=NEW.account_id AND reason='ACCOUNT_DELETION'))) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='account not available for membership';
  END IF;
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.state IN ('INVITED','ACTIVE') AND (NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE')) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='organization not accepting membership';
 END IF;
 SELECT state INTO employee_state FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
 IF employee_state IS NULL THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'INVITED' OR NEW.revision<>1 OR NEW.account_tombstone IS NOT NULL OR employee_state<>'ACTIVE' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='new membership must be invited';
  END IF;
  NEW.created_at=clock_timestamp();NEW.started_at=NULL;NEW.ended_at=NULL;NEW.tombstoned_at=NULL;
 ELSE
  IF OLD.state='ENDED' THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='membership period ended'; END IF;
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR NEW.employee_id<>OLD.employee_id OR NEW.access_model<>OLD.access_model
   OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1
   OR NEW.account_tombstone IS DISTINCT FROM OLD.account_tombstone
   OR (OLD.account_id IS NOT NULL AND NEW.account_id IS DISTINCT FROM OLD.account_id) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid membership revision or identity';
  END IF;
  IF NEW.state<>OLD.state AND NOT((OLD.state='INVITED' AND NEW.state IN ('ACTIVE','ENDED'))
   OR (OLD.state='ACTIVE' AND NEW.state IN ('SUSPENDED','ENDED')) OR (OLD.state='SUSPENDED' AND NEW.state IN ('ACTIVE','ENDED'))) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='membership transition not allowed';
  END IF;
  IF OLD.state IN ('ACTIVE','SUSPENDED') AND NEW.state IS DISTINCT FROM OLD.state THEN
   PERFORM public.org_membership_require_policy(NEW.org_id,NEW.id,CASE WHEN NEW.state='ACTIVE' THEN 'ACTIVATE' ELSE 'REDUCE' END);
  ELSIF OLD.state='INVITED' AND NEW.state='ACTIVE' THEN
   PERFORM public.org_membership_require_policy(NEW.org_id,NEW.id,'ACTIVATE');
  END IF;
  IF OLD.state=NEW.state AND (OLD.state<>'INVITED' OR OLD.account_id IS NOT NULL OR NEW.account_id IS NULL
   OR NEW.suspension_origin<>OLD.suspension_origin OR NEW.end_reason<>OLD.end_reason) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='membership history cannot be rewritten';
  END IF;
  NEW.created_at=OLD.created_at;NEW.started_at=OLD.started_at;NEW.ended_at=OLD.ended_at;NEW.tombstoned_at=OLD.tombstoned_at;
  IF OLD.state='INVITED' AND NEW.state='ACTIVE' THEN NEW.started_at=clock_timestamp(); END IF;
  IF NEW.state='ENDED' THEN NEW.ended_at=clock_timestamp(); END IF;
 END IF;
 IF NEW.state IN ('INVITED','ACTIVE') AND employee_state<>'ACTIVE' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee not active';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_membership_revision BEFORE INSERT OR UPDATE ON org_memberships
 FOR EACH ROW EXECUTE FUNCTION org_membership_guard();

CREATE FUNCTION org_employee_identity_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ACTIVE' OR NEW.revision<>1 OR NEW.merged_into IS NOT NULL OR NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid employee identity';
  END IF;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.merged_into IS NOT NULL
   OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid employee revision or identity';
  END IF;
  IF NEW.state=OLD.state OR NOT((OLD.state='ACTIVE' AND NEW.state IN ('INACTIVE','TERMINATED'))
   OR (OLD.state='INACTIVE' AND NEW.state IN ('ACTIVE','TERMINATED')) OR (OLD.state='TERMINATED' AND NEW.state='ACTIVE')) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee transition not allowed';
  END IF;
  IF EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND employee_id=NEW.id
   AND ((NEW.state='INACTIVE' AND state IN ('INVITED','ACTIVE')) OR (NEW.state='TERMINATED' AND state<>'ENDED'))) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='membership effects must precede employee change';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_employee_identity_revision BEFORE INSERT OR UPDATE ON org_employee_identities
 FOR EACH ROW EXECUTE FUNCTION org_employee_identity_guard();

-- Narrow internal definer. No new login/BYPASSRLS role is needed. Explicit
-- owner verification prevents a moved function owner from seeing an empty set.
CREATE FUNCTION org_membership_check_account_exit(target UUID) RETURNS VOID
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE item RECORD;prior_org TEXT;prior_account TEXT;
BEGIN
 IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass)) THEN
  RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable';
 END IF;
 PERFORM 1 FROM public.accounts WHERE id=target FOR UPDATE;
 prior_org=current_setting('anpfuel.org_id',true);prior_account=current_setting('anpfuel.erasure_account',true);
 PERFORM set_config('anpfuel.erasure_account',target::text,true);
 FOR item IN SELECT DISTINCT org_id FROM public.org_memberships WHERE account_id=target ORDER BY org_id LOOP
  PERFORM set_config('anpfuel.org_id',item.org_id::text,true);
  PERFORM 1 FROM public.organizations WHERE org_id=item.org_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable'; END IF;
 END LOOP;
 FOR item IN SELECT org_id,id FROM public.org_memberships WHERE account_id=target AND state IN ('ACTIVE','SUSPENDED') ORDER BY org_id,id LOOP
  PERFORM set_config('anpfuel.org_id',item.org_id::text,true);
  PERFORM public.org_membership_require_policy(item.org_id,item.id,'REDUCE');
 END LOOP;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 PERFORM set_config('anpfuel.erasure_account',COALESCE(prior_account,''),true);
END $$;
REVOKE ALL ON FUNCTION org_membership_check_account_exit(UUID) FROM PUBLIC;

CREATE FUNCTION org_membership_account_erasure() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE prior_account TEXT;prior_org TEXT;item RECORD;
BEGIN
 IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass)) THEN
  RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable';
 END IF;
 PERFORM public.org_membership_check_account_exit(NEW.id);
 prior_account=current_setting('anpfuel.erasure_account',true);prior_org=current_setting('anpfuel.org_id',true);
 PERFORM set_config('anpfuel.erasure_account',NEW.id::text,true);
 -- Updates use ordinary org RLS, including SELECT visibility of the new
 -- unlinked tuple. The account-only exception is inventory/read-only.
 FOR item IN SELECT DISTINCT org_id FROM public.org_memberships WHERE account_id=NEW.id ORDER BY org_id LOOP
  PERFORM set_config('anpfuel.org_id',item.org_id::text,true);
  UPDATE public.org_memberships SET account_id=NULL,account_tombstone=gen_random_uuid(),tombstoned_at=clock_timestamp(),
   state='ENDED',end_reason=CASE WHEN state='ENDED' THEN end_reason ELSE 'ACCOUNT_DELETED' END,
   ended_at=COALESCE(ended_at,clock_timestamp()),revision=revision+1 WHERE org_id=item.org_id AND account_id=NEW.id;
 END LOOP;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 PERFORM set_config('anpfuel.erasure_account',COALESCE(prior_account,''),true);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION org_membership_account_erasure() FROM PUBLIC;
CREATE TRIGGER org_membership_account_erasure AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION org_membership_account_erasure();

-- Existing durable exit intent fences admission before any provider call.
-- This prevents last-owner refusal after personal billing cancellation.
CREATE FUNCTION org_membership_billing_exit_guard() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE prior_account TEXT;linked BOOLEAN;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.account_id<>OLD.account_id OR NEW.operation_key<>OLD.operation_key OR NEW.reason<>OLD.reason THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='billing exit identity is immutable';
  END IF;
  RETURN NEW;
 ELSIF TG_OP='DELETE' THEN
  IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass)) THEN
   RAISE EXCEPTION USING ERRCODE='P7502',MESSAGE='enterprise access policy unavailable';
  END IF;
  IF EXISTS(SELECT 1 FROM public.accounts WHERE id=OLD.account_id AND status<>'deleted') THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='account deletion reservation is retained';
  END IF;
  prior_account=current_setting('anpfuel.erasure_account',true);
  PERFORM set_config('anpfuel.erasure_account',OLD.account_id::text,true);
  SELECT EXISTS(SELECT 1 FROM public.org_memberships WHERE account_id=OLD.account_id) INTO linked;
  PERFORM set_config('anpfuel.erasure_account',COALESCE(prior_account,''),true);
  IF linked THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='account deletion reservation is retained'; END IF;
  RETURN OLD;
 END IF;
 PERFORM public.org_membership_check_account_exit(NEW.account_id);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION org_membership_billing_exit_guard() FROM PUBLIC;
CREATE TRIGGER org_membership_billing_exit_guard BEFORE INSERT ON payment_billing_exits
 FOR EACH ROW WHEN(NEW.reason='ACCOUNT_DELETION') EXECUTE FUNCTION org_membership_billing_exit_guard();
CREATE TRIGGER org_membership_billing_exit_identity BEFORE UPDATE OF account_id,operation_key,reason ON payment_billing_exits
 FOR EACH ROW EXECUTE FUNCTION org_membership_billing_exit_guard();
CREATE TRIGGER org_membership_billing_exit_retention BEFORE DELETE ON payment_billing_exits
 FOR EACH ROW WHEN(OLD.reason='ACCOUNT_DELETION') EXECUTE FUNCTION org_membership_billing_exit_guard();
