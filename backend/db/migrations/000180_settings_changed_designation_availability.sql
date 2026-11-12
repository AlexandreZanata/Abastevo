-- EMP-ORG-008 review correction. A command re-proves availability only for a
-- responsibility slot whose member it designates or changes. A designee whose
-- account later became unavailable (suspended account, pending account
-- deletion) no longer blocks unrelated changes; membership reductions still
-- clear designations, and onboarding/bootstrap keep their full check. Only the
-- availability condition below differs from 000178.
CREATE OR REPLACE FUNCTION org_settings_version_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
     IF (NEW.source<>'COMMAND' OR member IS DISTINCT FROM current_values #>> ARRAY['responsibilities',item.key,slot])
      AND NOT public.org_settings_member_available(NEW.org_id,member::uuid) THEN RAISE EXCEPTION USING ERRCODE='P8103',MESSAGE='settings member unavailable',HINT='responsibilities.'||item.key||'.'||slot;END IF;
    END IF;
   END LOOP;
  END LOOP;
 END IF;
 RETURN NEW;
END $$;

