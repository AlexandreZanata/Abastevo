-- EMP-IDN-005: single-use personal consent and explicit privileged admission.
-- Prior migrations/history remain unchanged. Opaque scope resolution grants no
-- organization authority, no proof and no public organization discovery.
ALTER TABLE org_invitations DROP CONSTRAINT org_invitations_cause_check,ADD CONSTRAINT org_invitations_cause_check CHECK(cause IN('','MANAGER_REVOKED','REPLACED','EXPIRED','CONTACT_CHANGED','EMPLOYEE_UNAVAILABLE','ISSUER_REDUCED','ORGANIZATION_ARCHIVED','RESERVATION_ENDED','PERSON_ACCEPTED','PERSON_DECLINED','TOO_MANY_ATTEMPTS'));
ALTER TABLE org_invitations ADD COLUMN failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK(failed_attempts BETWEEN 0 AND 5);
CREATE TABLE org_invitation_locators(invitation_id UUID PRIMARY KEY,org_id UUID NOT NULL,FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitations(org_id,id));
ALTER TABLE org_invitation_locators ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_locators FORCE ROW LEVEL SECURITY;
CREATE POLICY org_invitation_locator_read ON org_invitation_locators FOR SELECT USING(invitation_id=NULLIF(current_setting('anpfuel.invitation_locator',true),'')::uuid OR org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_invitation_locator_write ON org_invitation_locators FOR INSERT WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_invitation_locators FROM PUBLIC;
CREATE TRIGGER org_invitation_locator_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_invitation_locators FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
-- Actual opaque locators only, no fabricated acceptance or actor history.
ALTER TABLE org_invitations NO FORCE ROW LEVEL SECURITY;ALTER TABLE org_invitation_locators NO FORCE ROW LEVEL SECURITY;
INSERT INTO org_invitation_locators SELECT id,org_id FROM org_invitations;
ALTER TABLE org_invitations FORCE ROW LEVEL SECURITY;ALTER TABLE org_invitation_locators FORCE ROW LEVEL SECURITY;
CREATE FUNCTION org_invitation_register_locator() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 INSERT INTO public.org_invitation_locators(invitation_id,org_id)VALUES(NEW.id,NEW.org_id);RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_register_locator AFTER INSERT ON org_invitations FOR EACH ROW EXECUTE FUNCTION org_invitation_register_locator();
CREATE TABLE org_invitation_acceptances(
 org_id UUID NOT NULL,invitation_id UUID NOT NULL,membership_id UUID NOT NULL,account_id UUID NOT NULL REFERENCES accounts(id),account_alias TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN('WAITING','ACTIVE','REJECTED','EXPIRED','ENDED')),revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 accepted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),deadline TIMESTAMPTZ NOT NULL,changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 confirmation_required BOOLEAN NOT NULL,settings_revision BIGINT NOT NULL,confirmed_by UUID,cause TEXT NOT NULL DEFAULT '',source_type TEXT NOT NULL DEFAULT '',source_id UUID,source_revision BIGINT,
 CHECK((state='ENDED' AND source_type IN('PROFILE','EMPLOYEE','MEMBERSHIP','ORGANIZATION') AND source_id IS NOT NULL AND source_revision>0) OR(state<>'ENDED' AND source_type='' AND source_id IS NULL AND source_revision IS NULL)),
 PRIMARY KEY(org_id,invitation_id),UNIQUE(org_id,membership_id),
 FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitations(org_id,id),FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,confirmed_by) REFERENCES org_memberships(org_id,id),FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision),
 CHECK(isfinite(accepted_at) AND isfinite(deadline) AND isfinite(changed_at) AND deadline=accepted_at+interval '168 hours' AND changed_at>=accepted_at),
 CHECK(length(account_alias) BETWEEN 1 AND 120),CHECK((state='WAITING' AND revision=1 AND cause='') OR(state<>'WAITING' AND revision>1 AND cause<>''))
);
CREATE TABLE org_invitation_person_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,actor_account_id UUID NOT NULL REFERENCES accounts(id),invitation_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('ACCEPT','DECLINE','CONFIRM','REJECT')),request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=8192),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitations(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE org_invitation_acceptance_notices(
 org_id UUID NOT NULL,invitation_id UUID NOT NULL,revision BIGINT NOT NULL,state TEXT NOT NULL,cause TEXT NOT NULL,occurred_at TIMESTAMPTZ NOT NULL,source_type TEXT NOT NULL,source_id UUID,source_revision BIGINT,
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,invitation_id,revision),FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitation_acceptances(org_id,invitation_id)
);
ALTER TABLE org_invitation_acceptances ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_acceptances FORCE ROW LEVEL SECURITY;
ALTER TABLE org_invitation_person_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_person_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_invitation_acceptance_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_acceptance_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_invitation_acceptance_scope ON org_invitation_acceptances USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_invitation_person_command_scope ON org_invitation_person_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_invitation_acceptance_notice_scope ON org_invitation_acceptance_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_invitation_acceptances,org_invitation_person_commands,org_invitation_acceptance_notices FROM PUBLIC;
CREATE TRIGGER org_invitation_acceptance_history BEFORE DELETE OR TRUNCATE ON org_invitation_acceptances FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_invitation_person_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_invitation_person_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_invitation_acceptance_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_invitation_acceptance_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_invitation_acceptance_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE at TIMESTAMPTZ;BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;at=clock_timestamp();
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
CREATE TRIGGER org_invitation_acceptance_guard BEFORE INSERT OR UPDATE ON org_invitation_acceptances FOR EACH ROW EXECUTE FUNCTION org_invitation_acceptance_guard();
CREATE FUNCTION org_invitation_acceptance_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 INSERT INTO public.org_invitation_acceptance_notices(org_id,invitation_id,revision,state,cause,occurred_at,source_type,source_id,source_revision)VALUES(NEW.org_id,NEW.invitation_id,NEW.revision,NEW.state,NEW.cause,NEW.changed_at,NEW.source_type,NEW.source_id,NEW.source_revision);
 IF NEW.state IN('REJECTED','EXPIRED','ENDED') THEN UPDATE public.org_memberships SET state='ENDED',revision=revision+1,end_reason='ADMISSION_ENDED' WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='INVITED';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_acceptance_project AFTER INSERT OR UPDATE ON org_invitation_acceptances FOR EACH ROW EXECUTE FUNCTION org_invitation_acceptance_project();
CREATE FUNCTION org_invitation_acceptance_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_invitation_acceptances a WHERE(a.org_id,a.invitation_id,a.revision,a.state,a.cause,a.changed_at)=(NEW.org_id,NEW.invitation_id,NEW.revision,NEW.state,NEW.cause,NEW.occurred_at) AND a.source_type=NEW.source_type AND a.source_id IS NOT DISTINCT FROM NEW.source_id AND a.source_revision IS NOT DISTINCT FROM NEW.source_revision) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_acceptance_notice_guard BEFORE INSERT ON org_invitation_acceptance_notices FOR EACH ROW EXECUTE FUNCTION org_invitation_acceptance_notice_guard();
CREATE FUNCTION org_invitation_person_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NEW.result-ARRAY['organization_id','invitation_id','membership_id','state','membership_state','accepted_at','confirmation_deadline','alias','revision']<>'{}'::jsonb OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_type='invitation' AND a.target_id=NEW.invitation_id AND a.request_hash=NEW.request_hash AND a.action=CASE NEW.kind WHEN 'ACCEPT' THEN 'idn.invitation.accepted' WHEN 'DECLINE' THEN 'idn.invitation.declined' WHEN 'CONFIRM' THEN 'idn.invitation.confirmed' ELSE 'idn.invitation.rejected' END) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='command source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_person_command_guard BEFORE INSERT ON org_invitation_person_commands FOR EACH ROW EXECUTE FUNCTION org_invitation_person_command_guard();
CREATE OR REPLACE FUNCTION org_invitation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;at TIMESTAMPTZ;days INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='invitation source unavailable';END IF;at=clock_timestamp();
 IF TG_OP='INSERT' THEN
  SELECT(settings_values->'policies'->>'invitation_validity_days')::integer INTO days FROM public.org_settings_versions WHERE org_id=NEW.org_id AND revision=NEW.settings_revision;
  IF root_state NOT IN('PENDING_SETUP','ACTIVE') OR NEW.failed_attempts<>0 OR NEW.state<>'PENDING' OR NEW.revision<>1 OR NEW.cause<>'' OR NEW.source_type<>''
   OR NEW.validity_days IS DISTINCT FROM days OR NOT EXISTS(SELECT 1 FROM public.org_settings_heads WHERE org_id=NEW.org_id AND revision=NEW.settings_revision)
   OR NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id AND state='ACTIVE')
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id AND employee_id=NEW.employee_id AND state='INVITED' AND account_id IS NULL AND access_model='EXISTING_ACCOUNT')
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id) WHERE m.org_id=NEW.org_id AND m.id=NEW.issuer_membership_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND m.employee_id<>NEW.employee_id)
   THEN RAISE EXCEPTION USING ERRCODE='P8801',MESSAGE='invalid invitation';END IF;
  NEW.created_at=at;NEW.changed_at=at;NEW.expires_at=at+NEW.validity_days*interval '24 hours';RETURN NEW;
 END IF;
 IF NEW.failed_attempts IS DISTINCT FROM OLD.failed_attempts THEN
  IF OLD.state<>'PENDING' OR at>=OLD.expires_at OR NEW.failed_attempts<>OLD.failed_attempts+1 OR NEW.failed_attempts>5 THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='invalid invitation attempt';END IF;
  IF NEW.failed_attempts<5 THEN
   IF (to_jsonb(NEW)-'failed_attempts') IS DISTINCT FROM (to_jsonb(OLD)-'failed_attempts') THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='invalid invitation attempt';END IF;
   RETURN NEW;
  ELSIF NEW.state<>'REVOKED' OR NEW.cause<>'TOO_MANY_ATTEMPTS' THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='invalid lockout';END IF;
 END IF;
 IF OLD.state<>'PENDING' OR NEW.state NOT IN('EXPIRED','REVOKED','SUPERSEDED','ACCEPTED','DECLINED') OR NEW.revision<>OLD.revision+1
  OR ROW(NEW.org_id,NEW.id,NEW.employee_id,NEW.membership_id,NEW.issuer_membership_id,NEW.operation_id,NEW.verifier,NEW.grants,NEW.validity_days,NEW.settings_revision,NEW.created_at,NEW.expires_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.employee_id,OLD.membership_id,OLD.issuer_membership_id,OLD.operation_id,OLD.verifier,OLD.grants,OLD.validity_days,OLD.settings_revision,OLD.created_at,OLD.expires_at) THEN RAISE EXCEPTION USING ERRCODE='P8803',MESSAGE='invitation history immutable';END IF;
 IF NEW.state IN('ACCEPTED','DECLINED') THEN
  IF at>=OLD.expires_at OR NEW.cause<>(CASE WHEN NEW.state='ACCEPTED' THEN 'PERSON_ACCEPTED' ELSE 'PERSON_DECLINED' END) OR NEW.source_type<>'' OR (NEW.state='ACCEPTED' AND NOT EXISTS(SELECT 1 FROM public.org_invitation_acceptances a JOIN public.org_memberships m ON(m.org_id,m.id)=(a.org_id,a.membership_id) WHERE a.org_id=NEW.org_id AND a.invitation_id=NEW.id AND a.membership_id=NEW.membership_id AND m.account_id=a.account_id AND m.state IN('INVITED','ACTIVE'))) THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='invalid redemption source';END IF;
 ELSIF NEW.cause='TOO_MANY_ATTEMPTS' THEN
  IF NEW.state<>'REVOKED' OR NEW.failed_attempts<>5 OR NEW.source_type<>'' THEN RAISE EXCEPTION USING ERRCODE='P8901',MESSAGE='invalid lockout';END IF;
 ELSIF NEW.state='EXPIRED' THEN
  IF at<OLD.expires_at OR NEW.cause<>'EXPIRED' OR NEW.source_type<>'' THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='expiration source unavailable';END IF;
 ELSIF NEW.state='SUPERSEDED' THEN
  IF NEW.cause<>'REPLACED' OR NEW.source_type<>'' THEN RAISE EXCEPTION USING ERRCODE='P8801',MESSAGE='invalid replacement';END IF;
 ELSIF NEW.cause='MANAGER_REVOKED' THEN
  IF NEW.source_type<>'' THEN RAISE EXCEPTION USING ERRCODE='P8801',MESSAGE='invalid manager source';END IF;
 ELSIF NEW.cause='CONTACT_CHANGED' THEN
  IF NEW.source_type<>'PROFILE' OR NEW.source_id<>NEW.employee_id OR NOT EXISTS(SELECT 1 FROM public.org_employee_profiles WHERE org_id=NEW.org_id AND employee_id=NEW.source_id AND revision=NEW.source_revision) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='contact source unavailable';END IF;
 ELSIF NEW.cause='EMPLOYEE_UNAVAILABLE' THEN
  IF NEW.source_type<>'EMPLOYEE' OR NEW.source_id<>NEW.employee_id OR NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.source_id AND revision=NEW.source_revision AND state IN('INACTIVE','TERMINATED')) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='employee source unavailable';END IF;
 ELSIF NEW.cause IN('ISSUER_REDUCED','RESERVATION_ENDED') THEN
  IF NEW.source_type<>'MEMBERSHIP' OR(NEW.cause='ISSUER_REDUCED' AND NEW.source_id<>NEW.issuer_membership_id) OR(NEW.cause='RESERVATION_ENDED' AND NEW.source_id<>NEW.membership_id)
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.source_id AND revision=NEW.source_revision AND state IN('SUSPENDED','ENDED')) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='membership source unavailable';END IF;
 ELSIF NEW.cause='ORGANIZATION_ARCHIVED' THEN
  IF NEW.source_type<>'ORGANIZATION' OR NEW.source_id<>NEW.org_id OR NOT EXISTS(SELECT 1 FROM public.organizations WHERE org_id=NEW.org_id AND revision=NEW.source_revision AND state IN('ARCHIVED','CLOSED')) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='organization source unavailable';END IF;
 ELSE RAISE EXCEPTION USING ERRCODE='P8801',MESSAGE='invalid invitation cause';END IF;
 NEW.changed_at=at;RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_invitation_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF TG_OP='UPDATE' AND NEW.revision=OLD.revision THEN RETURN NEW;END IF;
 INSERT INTO public.org_invitation_notices(org_id,invitation_id,revision,kind,occurred_at,cause,source_type,source_id,source_revision)VALUES(NEW.org_id,NEW.id,NEW.revision,NEW.state,NEW.changed_at,NEW.cause,NEW.source_type,NEW.source_id,NEW.source_revision);RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_invitation_context_changed() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_TABLE_NAME='org_employee_profiles' THEN
  UPDATE public.org_invitation_acceptances a SET state='ENDED',revision=a.revision+1,cause='CONTACT_CHANGED',source_type='PROFILE',source_id=NEW.employee_id,source_revision=NEW.revision FROM public.org_invitations i WHERE(a.org_id,a.invitation_id)=(i.org_id,i.id) AND i.org_id=NEW.org_id AND i.employee_id=NEW.employee_id AND a.state='WAITING';
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='CONTACT_CHANGED',source_type='PROFILE',source_id=NEW.employee_id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND employee_id=NEW.employee_id AND state='PENDING';
 ELSIF TG_TABLE_NAME='org_employee_identities' THEN
  UPDATE public.org_invitation_acceptances a SET state='ENDED',revision=a.revision+1,cause='EMPLOYEE_UNAVAILABLE',source_type='EMPLOYEE',source_id=NEW.id,source_revision=NEW.revision FROM public.org_invitations i WHERE(a.org_id,a.invitation_id)=(i.org_id,i.id) AND i.org_id=NEW.org_id AND i.employee_id=NEW.id AND a.state='WAITING';
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='EMPLOYEE_UNAVAILABLE',source_type='EMPLOYEE',source_id=NEW.id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND employee_id=NEW.id AND state='PENDING';
 ELSIF TG_TABLE_NAME='org_memberships' THEN
  UPDATE public.org_invitation_acceptances a SET state='ENDED',revision=a.revision+1,cause=CASE WHEN i.issuer_membership_id=NEW.id THEN 'ISSUER_REDUCED' ELSE 'RESERVATION_ENDED' END,source_type='MEMBERSHIP',source_id=NEW.id,source_revision=NEW.revision FROM public.org_invitations i WHERE(a.org_id,a.invitation_id)=(i.org_id,i.id) AND i.org_id=NEW.org_id AND (i.issuer_membership_id=NEW.id OR i.membership_id=NEW.id) AND a.state='WAITING';
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause=CASE WHEN issuer_membership_id=NEW.id THEN 'ISSUER_REDUCED' ELSE 'RESERVATION_ENDED' END,source_type='MEMBERSHIP',source_id=NEW.id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND(issuer_membership_id=NEW.id OR membership_id=NEW.id) AND state='PENDING';
 ELSE
  UPDATE public.org_invitation_acceptances SET state='ENDED',revision=revision+1,cause='ORGANIZATION_ARCHIVED',source_type='ORGANIZATION',source_id=NEW.org_id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND state='WAITING';
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='ORGANIZATION_ARCHIVED',source_type='ORGANIZATION',source_id=NEW.org_id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND state='PENDING';
 END IF;RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_settings_values_valid(v JSONB) RETURNS BOOLEAN
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p JSONB;r JSONB;pair JSONB;item RECORD;k TEXT;raw TEXT;n BIGINT;
BEGIN
 IF v IS NULL OR jsonb_typeof(v)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(v))<>7
  OR NOT v ?& ARRAY['time_zone','consumption_display','review_required','approval_levels_default','closure_period_days','policies','responsibilities'] THEN RETURN false;END IF;
 IF jsonb_typeof(v->'time_zone')<>'string' OR length(v->>'time_zone')>128 OR v->>'time_zone' !~ '^[A-Za-z0-9_+/-]+$'
  OR(v->>'time_zone'<>'UTC' AND position('/' IN(v->>'time_zone'))=0) OR v->>'time_zone' ~ '^(posix/|right/|SystemV/)' OR v->>'time_zone' ~ '(^/|/$|//)'
  OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=v->>'time_zone') THEN RETURN false;END IF;
 IF jsonb_typeof(v->'consumption_display')<>'string' OR v->>'consumption_display' NOT IN('KM_PER_L','L_PER_100KM') THEN RETURN false;END IF;
 r=v->'review_required';IF jsonb_typeof(r)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(r))<>3 OR NOT r ?& ARRAY['trips','fuelings','expenses'] THEN RETURN false;END IF;
 FOR k IN SELECT unnest(ARRAY['trips','fuelings','expenses']) LOOP IF jsonb_typeof(r->k)<>'boolean' THEN RETURN false;END IF;END LOOP;
 FOR k IN SELECT unnest(ARRAY['approval_levels_default','closure_period_days']) LOOP
  IF jsonb_typeof(v->k)<>'number' OR v->>k !~ '^[0-9]{1,2}$' THEN RETURN false;END IF;
 END LOOP;
 IF(v->>'approval_levels_default')::int NOT BETWEEN 1 AND 3 OR(v->>'closure_period_days')::int NOT BETWEEN 7 AND 90 THEN RETURN false;END IF;
 p=v->'policies';IF jsonb_typeof(p)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(p)) NOT IN(5,6) OR NOT p ?& ARRAY['intake_window_days','clock_skew_limit_seconds','delegation_maximum_days','invitation_validity_days','activation_validity_hours'] THEN RETURN false;END IF;
 IF p-ARRAY['intake_window_days','clock_skew_limit_seconds','delegation_maximum_days','invitation_validity_days','activation_validity_hours','require_invitation_confirmation']<>'{}'::jsonb OR(p ? 'require_invitation_confirmation' AND jsonb_typeof(p->'require_invitation_confirmation')<>'boolean') THEN RETURN false;END IF;
 FOR k IN SELECT unnest(ARRAY['intake_window_days','delegation_maximum_days','invitation_validity_days','activation_validity_hours']) LOOP
  IF jsonb_typeof(p->k)<>'number' OR p->>k !~ '^[0-9]{1,3}$' THEN RETURN false;END IF;
 END LOOP;
 IF(p->>'intake_window_days')::int NOT BETWEEN 0 AND 60 OR(p->>'delegation_maximum_days')::int NOT BETWEEN 1 AND 180
  OR(p->>'invitation_validity_days')::int NOT BETWEEN 1 AND 30 OR(p->>'activation_validity_hours')::int NOT BETWEEN 1 AND 168 THEN RETURN false;END IF;
 raw=p->>'clock_skew_limit_seconds';IF jsonb_typeof(p->'clock_skew_limit_seconds')<>'string' OR raw !~ '^(0|[1-9][0-9]{0,18})$' OR(length(raw)=19 AND raw>'9223372036854775807') THEN RETURN false;END IF;
 r=v->'responsibilities';IF jsonb_typeof(r)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(r))<>4 OR NOT r ?& ARRAY['review_fallback','platform_notices','data_protection','billing'] THEN RETURN false;END IF;
 FOR item IN SELECT * FROM jsonb_each(r) LOOP
  pair=item.value;IF jsonb_typeof(pair)<>'object' OR(SELECT count(*) FROM jsonb_object_keys(pair))<>2 OR NOT pair ?& ARRAY['primary','substitute'] THEN RETURN false;END IF;
  FOR k IN SELECT unnest(ARRAY['primary','substitute']) LOOP
   raw=pair->>k;IF jsonb_typeof(pair->k)<>'string' OR(raw<>'' AND(raw !~ '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$' OR raw='00000000-0000-0000-0000-000000000000')) THEN RETURN false;END IF;
  END LOOP;
  IF pair->>'primary'<>'' AND pair->>'primary'=pair->>'substitute' THEN RETURN false;END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN invalid_text_representation OR numeric_value_out_of_range THEN RETURN false;
END $$;

DO $$ DECLARE owner_name TEXT;BEGIN
 FOR owner_name IN SELECT DISTINCT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid IN('public.org_memberships'::regclass,'public.org_employee_profiles'::regclass,'public.org_employee_identities'::regclass,'public.organizations'::regclass) LOOP
  EXECUTE pg_catalog.format('GRANT SELECT,UPDATE ON public.org_invitation_acceptances TO %I',owner_name);
  EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON public.org_invitation_acceptance_notices TO %I',owner_name);
 END LOOP;END $$;

-- Preserve the earlier immutable receipt whitelist while allowing the bounded
-- failed-attempt metadata in new snapshots (never a verifier or raw secret).
CREATE OR REPLACE FUNCTION org_invitation_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NEW.result-ARRAY['failed_attempts','id','employee_id','membership_id','issuer_membership_id','operation_id','state','revision','settings_revision','validity_days','created_at','expires_at','changed_at','grants','cause','source_type','source_id','source_revision']<>'{}'::jsonb
  OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_id=NEW.invitation_id AND a.target_type='invitation' AND a.request_hash=NEW.request_hash AND((NEW.kind IN('ISSUE','RESEND') AND a.action='idn.invitation.issued') OR(NEW.kind='REVOKE' AND a.action='idn.invitation.revoked'))) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='invitation command source unavailable';END IF;RETURN NEW;
END $$;

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
 -- Ending a bound, never-active acceptance changes no account or role. Its
 -- actual terminal acceptance is already serialized by the root. Do not take
 -- an account lock after that root (contact/issuer/expiry projections).
 IF TG_OP='UPDATE' AND OLD.state='INVITED' AND NEW.state='ENDED' AND NEW.end_reason='ADMISSION_ENDED'
  AND EXISTS(SELECT 1 FROM public.org_invitation_acceptances WHERE org_id=OLD.org_id AND membership_id=OLD.id AND account_id=OLD.account_id AND state IN('REJECTED','EXPIRED','ENDED')) THEN
  IF OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1
   OR (to_jsonb(NEW)-ARRAY['state','revision','end_reason','ended_at']) IS DISTINCT FROM(to_jsonb(OLD)-ARRAY['state','revision','end_reason','ended_at'])
   OR EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=OLD.org_id AND membership_id=OLD.id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid admission reduction';END IF;
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  NEW.ended_at=clock_timestamp();RETURN NEW;
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
