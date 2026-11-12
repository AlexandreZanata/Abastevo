-- EMP-IDN-004: manager-mediated invitation reservations, never account access.
CREATE FUNCTION org_invitation_grants_valid(value JSONB) RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog AS $$
DECLARE item JSONB;seen TEXT[]='{}';k TEXT;
BEGIN
 IF value IS NULL OR jsonb_typeof(value)<>'array' OR jsonb_array_length(value) NOT BETWEEN 1 AND 10 THEN RETURN false;END IF;
 FOR item IN SELECT jsonb_array_elements(value) LOOP
  IF jsonb_typeof(item)<>'object' OR NOT(item ? 'role' AND item ? 'scope') OR item-ARRAY['role','scope']<>'{}'::jsonb
   OR item->>'role' IS NULL OR item->>'scope' IS NULL OR item->>'role' NOT IN('OWNER','ADMIN','FLEET_MANAGER','SUPERVISOR','FINANCE','APPROVER','DRIVER','OPERATIONS_STAFF','AUDITOR','DATA_MANAGER')
   OR item->>'scope' NOT IN('ORG','SELF') OR(item->>'role'='OWNER' AND item->>'scope'<>'ORG') THEN RETURN false;END IF;
  k=(item->>'role')||':'||(item->>'scope');IF k=ANY(seen) THEN RETURN false;END IF;seen=array_append(seen,k);
 END LOOP;RETURN true;
END $$;
CREATE TABLE org_invitations(
 org_id UUID NOT NULL, id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 employee_id UUID NOT NULL,membership_id UUID NOT NULL,issuer_membership_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 verifier TEXT NOT NULL CHECK(verifier ~ '^[a-f0-9]{64}$'),
 state TEXT NOT NULL CHECK(state IN('PENDING','EXPIRED','REVOKED','SUPERSEDED','ACCEPTED','DECLINED')),
 grants JSONB NOT NULL CHECK(org_invitation_grants_valid(grants)),
 validity_days INTEGER NOT NULL CHECK(validity_days BETWEEN 1 AND 30),settings_revision BIGINT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),expires_at TIMESTAMPTZ NOT NULL,changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 cause TEXT NOT NULL DEFAULT '' CHECK(cause IN('','MANAGER_REVOKED','REPLACED','EXPIRED','CONTACT_CHANGED','EMPLOYEE_UNAVAILABLE','ISSUER_REDUCED','ORGANIZATION_ARCHIVED','RESERVATION_ENDED')),
 source_type TEXT NOT NULL DEFAULT '' CHECK(source_type IN('','PROFILE','EMPLOYEE','MEMBERSHIP','ORGANIZATION')),source_id UUID,source_revision BIGINT,
 PRIMARY KEY(org_id,id),UNIQUE(org_id,operation_id),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,issuer_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision),
 CHECK(isfinite(created_at) AND isfinite(expires_at) AND isfinite(changed_at)),
 CHECK(expires_at=created_at+validity_days*interval '24 hours' AND changed_at>=created_at),
 CHECK((state='PENDING' AND revision=1 AND cause='' AND changed_at=created_at) OR(state<>'PENDING' AND revision>1 AND cause<>'')),
 CHECK((source_type='' AND source_id IS NULL AND source_revision IS NULL) OR(source_type<>'' AND source_id IS NOT NULL AND source_revision IS NOT NULL AND source_revision>0))
);
CREATE UNIQUE INDEX org_invitations_one_pending ON org_invitations(org_id,employee_id) WHERE state='PENDING';
CREATE INDEX org_invitations_expiry ON org_invitations(org_id,expires_at) WHERE state='PENDING';
CREATE INDEX org_invitations_issuer ON org_invitations(org_id,issuer_membership_id) WHERE state='PENDING';
CREATE TABLE org_invitation_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,actor_account_id UUID NOT NULL REFERENCES accounts(id),
 invitation_id UUID NOT NULL,kind TEXT NOT NULL CHECK(kind IN('ISSUE','RESEND','REVOKE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=16384),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitations(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE org_invitation_notices(
 org_id UUID NOT NULL,invitation_id UUID NOT NULL,revision BIGINT NOT NULL,kind TEXT NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,cause TEXT NOT NULL,source_type TEXT NOT NULL,source_id UUID,source_revision BIGINT,
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,invitation_id,revision),FOREIGN KEY(org_id,invitation_id) REFERENCES org_invitations(org_id,id)
);
ALTER TABLE org_invitations ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitations FORCE ROW LEVEL SECURITY;
ALTER TABLE org_invitation_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_invitation_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_invitation_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_invitation_scope ON org_invitations USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_invitation_command_scope ON org_invitation_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_invitation_notice_scope ON org_invitation_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_invitations,org_invitation_commands,org_invitation_notices FROM PUBLIC;
CREATE TRIGGER org_invitation_history BEFORE DELETE OR TRUNCATE ON org_invitations FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_invitation_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_invitation_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_invitation_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_invitation_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_invitation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;at TIMESTAMPTZ;days INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='invitation source unavailable';END IF;at=clock_timestamp();
 IF TG_OP='INSERT' THEN
  SELECT(settings_values->'policies'->>'invitation_validity_days')::integer INTO days FROM public.org_settings_versions WHERE org_id=NEW.org_id AND revision=NEW.settings_revision;
  IF root_state NOT IN('PENDING_SETUP','ACTIVE') OR NEW.state<>'PENDING' OR NEW.revision<>1 OR NEW.cause<>'' OR NEW.source_type<>''
   OR NEW.validity_days IS DISTINCT FROM days OR NOT EXISTS(SELECT 1 FROM public.org_settings_heads WHERE org_id=NEW.org_id AND revision=NEW.settings_revision)
   OR NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id AND state='ACTIVE')
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id AND employee_id=NEW.employee_id AND state='INVITED' AND account_id IS NULL AND access_model='EXISTING_ACCOUNT')
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id) WHERE m.org_id=NEW.org_id AND m.id=NEW.issuer_membership_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND m.employee_id<>NEW.employee_id)
   THEN RAISE EXCEPTION USING ERRCODE='P8801',MESSAGE='invalid invitation';END IF;
  NEW.created_at=at;NEW.changed_at=at;NEW.expires_at=at+NEW.validity_days*interval '24 hours';RETURN NEW;
 END IF;
 IF OLD.state<>'PENDING' OR NEW.state NOT IN('EXPIRED','REVOKED','SUPERSEDED') OR NEW.revision<>OLD.revision+1
  OR ROW(NEW.org_id,NEW.id,NEW.employee_id,NEW.membership_id,NEW.issuer_membership_id,NEW.operation_id,NEW.verifier,NEW.grants,NEW.validity_days,NEW.settings_revision,NEW.created_at,NEW.expires_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.employee_id,OLD.membership_id,OLD.issuer_membership_id,OLD.operation_id,OLD.verifier,OLD.grants,OLD.validity_days,OLD.settings_revision,OLD.created_at,OLD.expires_at) THEN RAISE EXCEPTION USING ERRCODE='P8803',MESSAGE='invitation history immutable';END IF;
 IF NEW.state='EXPIRED' THEN
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
CREATE TRIGGER org_invitation_guard BEFORE INSERT OR UPDATE ON org_invitations FOR EACH ROW EXECUTE FUNCTION org_invitation_guard();
CREATE FUNCTION org_invitation_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_invitations i WHERE i.org_id=NEW.org_id AND i.id=NEW.invitation_id AND i.revision=NEW.revision AND i.state=NEW.kind AND i.changed_at=NEW.occurred_at AND i.cause=NEW.cause AND i.source_type=NEW.source_type AND i.source_id IS NOT DISTINCT FROM NEW.source_id AND i.source_revision IS NOT DISTINCT FROM NEW.source_revision) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='invitation notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_notice_guard BEFORE INSERT ON org_invitation_notices FOR EACH ROW EXECUTE FUNCTION org_invitation_notice_guard();
CREATE FUNCTION org_invitation_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NEW.result-ARRAY['id','employee_id','membership_id','issuer_membership_id','operation_id','state','revision','settings_revision','validity_days','created_at','expires_at','changed_at','grants','cause','source_type','source_id','source_revision']<>'{}'::jsonb
  OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_id=NEW.invitation_id AND a.target_type='invitation' AND a.request_hash=NEW.request_hash AND((NEW.kind IN('ISSUE','RESEND') AND a.action='idn.invitation.issued') OR(NEW.kind='REVOKE' AND a.action='idn.invitation.revoked'))) THEN RAISE EXCEPTION USING ERRCODE='P8802',MESSAGE='invitation command source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_command_guard BEFORE INSERT ON org_invitation_commands FOR EACH ROW EXECUTE FUNCTION org_invitation_command_guard();
CREATE FUNCTION org_invitation_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 INSERT INTO public.org_invitation_notices(org_id,invitation_id,revision,kind,occurred_at,cause,source_type,source_id,source_revision)VALUES(NEW.org_id,NEW.id,NEW.revision,NEW.state,NEW.changed_at,NEW.cause,NEW.source_type,NEW.source_id,NEW.source_revision);RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_project AFTER INSERT OR UPDATE ON org_invitations FOR EACH ROW EXECUTE FUNCTION org_invitation_project();
-- Deferred cleanup observes the replacement in the same transaction, so a
-- resend cannot accidentally end its shared INVITED reservation in between.
CREATE FUNCTION org_invitation_finish_reservation() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_invitations WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id AND state='PENDING') THEN
  UPDATE public.org_memberships SET state='ENDED',revision=revision+1,end_reason='ADMISSION_ENDED' WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='INVITED' AND account_id IS NULL;
 END IF;RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER org_invitation_finish_reservation AFTER INSERT OR UPDATE ON org_invitations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_invitation_finish_reservation();
CREATE FUNCTION org_invitation_context_changed() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_TABLE_NAME='org_employee_profiles' THEN
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='CONTACT_CHANGED',source_type='PROFILE',source_id=NEW.employee_id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND employee_id=NEW.employee_id AND state='PENDING';
 ELSIF TG_TABLE_NAME='org_employee_identities' THEN
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='EMPLOYEE_UNAVAILABLE',source_type='EMPLOYEE',source_id=NEW.id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND employee_id=NEW.id AND state='PENDING';
 ELSIF TG_TABLE_NAME='org_memberships' THEN
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause=CASE WHEN issuer_membership_id=NEW.id THEN 'ISSUER_REDUCED' ELSE 'RESERVATION_ENDED' END,source_type='MEMBERSHIP',source_id=NEW.id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND(issuer_membership_id=NEW.id OR membership_id=NEW.id) AND state='PENDING';
 ELSE
  UPDATE public.org_invitations SET state='REVOKED',revision=revision+1,cause='ORGANIZATION_ARCHIVED',source_type='ORGANIZATION',source_id=NEW.org_id,source_revision=NEW.revision WHERE org_id=NEW.org_id AND state='PENDING';
 END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_invitation_contact_created AFTER INSERT ON org_employee_profiles FOR EACH ROW WHEN(NEW.work_email<>'' OR NEW.work_phone<>'') EXECUTE FUNCTION org_invitation_context_changed();
CREATE TRIGGER org_invitation_contact AFTER UPDATE OF work_email,work_phone ON org_employee_profiles FOR EACH ROW WHEN(NEW.work_email IS DISTINCT FROM OLD.work_email OR NEW.work_phone IS DISTINCT FROM OLD.work_phone) EXECUTE FUNCTION org_invitation_context_changed();
CREATE TRIGGER org_invitation_employee AFTER UPDATE OF state ON org_employee_identities FOR EACH ROW WHEN(NEW.state IN('INACTIVE','TERMINATED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_invitation_context_changed();
CREATE TRIGGER org_invitation_issuer AFTER UPDATE OF state ON org_memberships FOR EACH ROW WHEN(NEW.state IN('SUSPENDED','ENDED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_invitation_context_changed();
CREATE TRIGGER org_invitation_organization AFTER UPDATE OF state ON organizations FOR EACH ROW WHEN(NEW.state IN('ARCHIVED','CLOSED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_invitation_context_changed();
DO $$ DECLARE owner_name TEXT;BEGIN
 FOR owner_name IN SELECT DISTINCT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid IN('public.org_memberships'::regclass,'public.org_employee_profiles'::regclass,'public.org_employee_identities'::regclass,'public.organizations'::regclass) LOOP
  EXECUTE pg_catalog.format('GRANT SELECT,UPDATE ON public.org_invitations TO %I',owner_name);
  EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON public.org_invitation_notices TO %I',owner_name);
 END LOOP;
END $$;
