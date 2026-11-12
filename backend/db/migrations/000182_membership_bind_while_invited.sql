-- EMP-IDN-003 review correction. An account is bound only by an explicit
-- INVITED -> INVITED revision. Ending or activating an unbound invitation can no
-- longer attach an account in the same step. Only the identity condition below
-- differs from 000170; the erasure branch and transitions are unchanged.
CREATE OR REPLACE FUNCTION org_membership_guard() RETURNS trigger
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
   OR (OLD.account_id IS NOT NULL AND NEW.account_id IS DISTINCT FROM OLD.account_id)
   OR (OLD.account_id IS NULL AND NEW.account_id IS NOT NULL AND (OLD.state<>'INVITED' OR NEW.state<>'INVITED')) THEN
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

