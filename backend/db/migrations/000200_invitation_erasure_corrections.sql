-- Enterprise backend review corrections (B-BR-EMP-170/172, EDR-123/125).
-- Applied migrations and their history remain unchanged; this file only adds.

-- B-BR-EMP-170: an acceptance loses its account reference and copied alias
-- only together. A live link keeps the original 1..120 alias bound.
ALTER TABLE org_invitation_acceptances ALTER COLUMN account_id DROP NOT NULL,
 DROP CONSTRAINT org_invitation_acceptances_account_alias_check,
 ADD CONSTRAINT org_invitation_acceptances_account_link_check CHECK((account_id IS NOT NULL AND length(account_alias) BETWEEN 1 AND 120) OR(account_id IS NULL AND account_alias=''));
CREATE OR REPLACE FUNCTION org_invitation_acceptance_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE at TIMESTAMPTZ;BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;at=clock_timestamp();
 -- B-BR-EMP-170: the only identity change is the erasure definer's unlink of an
 -- actually deleted account, exactly once, with every other column unchanged.
 IF TG_OP='UPDATE' AND ROW(NEW.account_id,NEW.account_alias) IS DISTINCT FROM ROW(OLD.account_id,OLD.account_alias) THEN
  IF current_user IS DISTINCT FROM pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='public.org_memberships'::regclass))
   OR OLD.account_id IS NULL OR OLD.account_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.erasure_account',true),'')::uuid
   OR NEW.account_id IS NOT NULL OR NEW.account_alias<>''
   OR(to_jsonb(NEW)-ARRAY['account_id','account_alias']) IS DISTINCT FROM(to_jsonb(OLD)-ARRAY['account_id','account_alias'])
   OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=OLD.account_id AND status='deleted') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid account unlink';
  END IF;
  RETURN NEW;
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'WAITING' OR NEW.revision<>1 OR NOT EXISTS(SELECT 1 FROM public.org_invitations i JOIN public.org_memberships m ON(m.org_id,m.id)=(i.org_id,i.membership_id) JOIN public.accounts a ON a.id=m.account_id WHERE i.org_id=NEW.org_id AND i.id=NEW.invitation_id AND i.membership_id=NEW.membership_id AND i.state='PENDING' AND i.expires_at>at AND m.state='INVITED' AND m.access_model='EXISTING_ACCOUNT' AND m.account_id=NEW.account_id AND a.status='active' AND a.enterprise_managed_org_id IS NULL AND a.alias=NEW.account_alias
 AND EXISTS(SELECT 1 FROM public.org_settings_heads h JOIN public.org_settings_versions sv ON(sv.org_id,sv.revision)=(h.org_id,h.revision)
 WHERE h.org_id=NEW.org_id AND h.revision=NEW.settings_revision AND NEW.confirmation_required=(COALESCE((sv.settings_values->'policies'->>'require_invitation_confirmation')::boolean,false) OR EXISTS(SELECT 1 FROM jsonb_array_elements(i.grants) g WHERE g->>'role' NOT IN('DRIVER','OPERATIONS_STAFF'))))) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='admission source unavailable';END IF;
  NEW.accepted_at=at;NEW.changed_at=at;NEW.deadline=at+interval '168 hours';RETURN NEW;
 END IF;
 IF OLD.state<>'WAITING' OR NEW.state NOT IN('ACTIVE','REJECTED','EXPIRED','ENDED') OR NEW.revision<>OLD.revision+1 OR ROW(NEW.org_id,NEW.invitation_id,NEW.membership_id,NEW.account_id,NEW.account_alias,NEW.accepted_at,NEW.deadline,NEW.confirmation_required,NEW.settings_revision) IS DISTINCT FROM ROW(OLD.org_id,OLD.invitation_id,OLD.membership_id,OLD.account_id,OLD.account_alias,OLD.accepted_at,OLD.deadline,OLD.confirmation_required,OLD.settings_revision) THEN RAISE EXCEPTION USING ERRCODE='P8902',MESSAGE='admission history immutable';END IF;
 IF NEW.state='ACTIVE' AND(at>=OLD.deadline OR NEW.cause NOT IN('AUTO_CONFIRMED','MANAGER_CONFIRMED') OR(OLD.confirmation_required AND NEW.confirmed_by IS NULL) OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='ACTIVE' AND account_id=NEW.account_id)) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='activation source unavailable';END IF;
 IF NEW.state='EXPIRED' AND(at<OLD.deadline OR NEW.cause<>'CONFIRMATION_EXPIRED') THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='expiry source unavailable';END IF;
 IF NEW.state='REJECTED' AND(NEW.cause<>'MANAGER_REJECTED' OR NEW.confirmed_by IS NULL) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='rejection source unavailable';END IF;
 IF NEW.state='ENDED' AND NEW.cause NOT IN('CONTACT_CHANGED','EMPLOYEE_UNAVAILABLE','ISSUER_REDUCED','RESERVATION_ENDED','ORGANIZATION_ARCHIVED') THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='reduction source unavailable';END IF;
 IF NEW.state='ENDED' AND NOT EXISTS(SELECT 1 FROM public.org_invitations i WHERE i.org_id=NEW.org_id AND i.id=NEW.invitation_id AND(
 (NEW.cause='CONTACT_CHANGED' AND NEW.source_type='PROFILE' AND NEW.source_id=i.employee_id AND EXISTS(SELECT 1 FROM public.org_employee_profiles WHERE org_id=NEW.org_id AND employee_id=NEW.source_id AND revision=NEW.source_revision)) OR
 (NEW.cause='EMPLOYEE_UNAVAILABLE' AND NEW.source_type='EMPLOYEE' AND NEW.source_id=i.employee_id AND EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.source_id AND revision=NEW.source_revision AND state IN('INACTIVE','TERMINATED'))) OR
 (NEW.cause IN('ISSUER_REDUCED','RESERVATION_ENDED') AND NEW.source_type='MEMBERSHIP' AND NEW.source_id=CASE WHEN NEW.cause='ISSUER_REDUCED' THEN i.issuer_membership_id ELSE i.membership_id END AND EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.source_id AND revision=NEW.source_revision AND state IN('SUSPENDED','ENDED'))) OR
 (NEW.cause='ORGANIZATION_ARCHIVED' AND NEW.source_type='ORGANIZATION' AND NEW.source_id=NEW.org_id AND EXISTS(SELECT 1 FROM public.organizations WHERE org_id=NEW.org_id AND revision=NEW.source_revision AND state IN('ARCHIVED','CLOSED'))))) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='reduction source unavailable';END IF;
 NEW.changed_at=at;RETURN NEW;
END $$;
-- The unlink keeps the revision; it is not an admission transition or notice.
CREATE OR REPLACE FUNCTION org_invitation_acceptance_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF TG_OP='UPDATE' AND NEW.revision=OLD.revision THEN RETURN NEW;END IF;
 INSERT INTO public.org_invitation_acceptance_notices(org_id,invitation_id,revision,state,cause,occurred_at,source_type,source_id,source_revision)VALUES(NEW.org_id,NEW.invitation_id,NEW.revision,NEW.state,NEW.cause,NEW.changed_at,NEW.source_type,NEW.source_id,NEW.source_revision);
 IF NEW.state IN('REJECTED','EXPIRED','ENDED') THEN UPDATE public.org_memberships SET state='ENDED',revision=revision+1,end_reason='ADMISSION_ENDED' WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='INVITED';END IF;RETURN NEW;
END $$;

-- Same 000170 definer, plus the acceptance unlink under each organization's
-- own scope after that organization's membership tombstone.
CREATE OR REPLACE FUNCTION org_membership_account_erasure() RETURNS trigger
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
  -- An acceptance's account always equals its membership's bound account, so
  -- the membership inventory reaches every acceptance of this account.
  UPDATE public.org_invitation_acceptances SET account_id=NULL,account_alias='' WHERE org_id=item.org_id AND account_id=NEW.id;
 END LOOP;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 PERFORM set_config('anpfuel.erasure_account',COALESCE(prior_account,''),true);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION org_membership_account_erasure() FROM PUBLIC;

-- B-BR-EMP-172: deferred cleanup may run after a caller (including the
-- erasure definer) restored an empty or different scope. The row's own scope
-- was proved when it was written; reinstall it only inside this invoker
-- check, then restore the caller's scope. No definer or RLS bypass.
CREATE OR REPLACE FUNCTION org_invitation_finish_reservation() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);
 PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_invitations WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id AND state='PENDING') THEN
  UPDATE public.org_memberships SET state='ENDED',revision=revision+1,end_reason='ADMISSION_ENDED' WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='INVITED' AND account_id IS NULL;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 RETURN NEW;
END $$;

-- Accounts deleted before this upgrade receive the same unlink through the
-- guarded erasure branch: the owner reads the cross-organization inventory
-- only while forced RLS is lifted inside this migration transaction.
ALTER TABLE org_invitation_acceptances NO FORCE ROW LEVEL SECURITY;
DO $$ DECLARE item RECORD;BEGIN
 FOR item IN SELECT DISTINCT a.org_id,a.account_id FROM public.org_invitation_acceptances a JOIN public.accounts x ON x.id=a.account_id WHERE x.status='deleted' ORDER BY a.org_id,a.account_id LOOP
  PERFORM pg_catalog.set_config('anpfuel.org_id',item.org_id::text,true);
  PERFORM pg_catalog.set_config('anpfuel.erasure_account',item.account_id::text,true);
  UPDATE public.org_invitation_acceptances SET account_id=NULL,account_alias='' WHERE org_id=item.org_id AND account_id=item.account_id;
 END LOOP;
 PERFORM pg_catalog.set_config('anpfuel.org_id','',true);
 PERFORM pg_catalog.set_config('anpfuel.erasure_account','',true);
END $$;
ALTER TABLE org_invitation_acceptances FORCE ROW LEVEL SECURITY;
