-- EMP-AUTH-002 review correction. A new role period may start up to five
-- minutes before the database clock (application/database skew) or later, and
-- must not already be over. History keeps old rows; only INSERT changes, and
-- only the period condition below differs from 000171.
CREATE OR REPLACE FUNCTION org_role_assignment_guard() RETURNS trigger
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
  -- A new period starts now (bounded app/database skew) or later and has not
  -- already ended: storage never fabricates retroactive authority history.
  IF NEW.valid_from<clock_timestamp()-interval '5 minutes' OR NEW.valid_to<=clock_timestamp() THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment period must start now or later';
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

