-- EMP-IDN-015. Organization enrolments and original scoped acknowledgements.
-- No personal family is owned/revoked by this registry; no runtime grants.
CREATE TABLE org_device_enrollments(
 org_id UUID NOT NULL,id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 membership_id UUID NOT NULL,key_id UUID NOT NULL REFERENCES identity_keys(id),
 contributor_id UUID NOT NULL REFERENCES identity_contributors(id),
 fingerprint TEXT NOT NULL CHECK(fingerprint~'^fp:[a-f0-9]{64}$'),
 label TEXT NOT NULL CHECK(octet_length(label) BETWEEN 1 AND 80 AND label=btrim(label) AND label!~'(^[[:space:]]|[[:space:]]$|[[:cntrl:]])'),
 state TEXT NOT NULL DEFAULT 'ENROLLED' CHECK(state IN('ENROLLED','REVOKED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 enrollment_limit INTEGER NOT NULL CHECK(enrollment_limit BETWEEN 1 AND 5),
 enrolled_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(enrolled_at)),
 last_sync_at TIMESTAMPTZ CHECK(last_sync_at IS NULL),
 revoked_at TIMESTAMPTZ CHECK(revoked_at IS NULL OR(isfinite(revoked_at) AND revoked_at>=enrolled_at)),
 revocation_class TEXT NOT NULL DEFAULT '' CHECK(revocation_class IN('','LOST_OR_COMPROMISED','ACCESS_ENDED')),
 source_type TEXT NOT NULL DEFAULT '' CHECK(source_type IN('','COMMAND','MEMBERSHIP')),
 source_id UUID,source_revision BIGINT NOT NULL DEFAULT 0 CHECK(source_revision>=0),
 created_operation_id UUID NOT NULL,
 PRIMARY KEY(org_id,id),UNIQUE(org_id,membership_id,key_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,created_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((state='ENROLLED' AND revoked_at IS NULL AND revocation_class='' AND source_type='' AND source_id IS NULL AND source_revision=0)
  OR(state='REVOKED' AND revoked_at IS NOT NULL AND revocation_class<>'' AND source_type<>'' AND source_id IS NOT NULL AND source_revision>0))
);
CREATE INDEX org_device_member_page ON org_device_enrollments(org_id,membership_id,id);
CREATE INDEX org_device_scoped_key ON org_device_enrollments(org_id,key_id,id);
CREATE TABLE org_device_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL REFERENCES accounts(id),actor_key_id UUID REFERENCES identity_keys(id),device_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('ENROLL','REVOKE')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 reason TEXT NOT NULL CHECK((kind='ENROLL' AND reason='')OR(kind='REVOKE' AND length(btrim(reason)) BETWEEN 1 AND 280 AND reason!~'[[:cntrl:]]')),
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=8192),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(occurred_at)),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,device_id) REFERENCES org_device_enrollments(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_device_enrollments ENABLE ROW LEVEL SECURITY;ALTER TABLE org_device_enrollments FORCE ROW LEVEL SECURITY;
ALTER TABLE org_device_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_device_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_device_enrollment_scope ON org_device_enrollments USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_device_command_scope ON org_device_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_device_enrollments,org_device_commands FROM PUBLIC;
CREATE TRIGGER org_device_history BEFORE DELETE OR TRUNCATE ON org_device_enrollments FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_device_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_device_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_device_enrollment_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;member_account UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ENROLLED' OR NEW.revision<>1 OR NEW.last_sync_at IS NOT NULL OR NEW.revoked_at IS NOT NULL
   OR NEW.revocation_class<>'' OR NEW.source_type<>'' OR NEW.source_id IS NOT NULL OR NEW.source_revision<>0
   OR NOT root_ready OR root_state NOT IN('PENDING_SETUP','ACTIVE')
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device enrollment unavailable';END IF;
  SELECT m.account_id INTO member_account FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
   JOIN public.accounts a ON a.id=m.account_id
   WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND m.state='ACTIVE' AND m.access_model='EXISTING_ACCOUNT'
    AND e.state='ACTIVE' AND e.merged_into IS NULL AND a.status='active' AND a.enterprise_managed_org_id IS NULL
    AND NOT EXISTS(SELECT 1 FROM public.payment_billing_exits p WHERE p.account_id=a.id AND p.reason='ACCOUNT_DELETION');
  IF member_account IS NULL OR NOT EXISTS(
   SELECT 1 FROM public.identity_keys k JOIN public.identity_contributors c ON c.id=k.contributor_id
   JOIN public.account_contributor_bindings b ON b.contributor_id=c.id::text
   WHERE k.id=NEW.key_id AND k.contributor_id=NEW.contributor_id AND k.fingerprint=NEW.fingerprint AND k.revoked_at IS NULL AND c.status='active' AND b.account_id=member_account)
   OR EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND key_id=NEW.key_id AND state='REVOKED' AND revocation_class='LOST_OR_COMPROMISED')
   OR(SELECT count(*) FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id AND state='ENROLLED')>=NEW.enrollment_limit
   OR NOT EXISTS(SELECT 1 FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.created_operation_id AND actor_id=member_account AND action='idn.device.enrolled' AND target_type='device' AND target_id=NEW.id AND reason_code='enrolled')
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device enrollment unavailable';END IF;
  NEW.enrolled_at=clock_timestamp();RETURN NEW;
 END IF;
 IF OLD.state='REVOKED' OR OLD.revision=9223372036854775807 OR NEW.state<>'REVOKED' OR NEW.revision<>OLD.revision+1
  OR(to_jsonb(NEW)-ARRAY['state','revision','revoked_at','revocation_class','source_type','source_id','source_revision']) IS DISTINCT FROM(to_jsonb(OLD)-ARRAY['state','revision','revoked_at','revocation_class','source_type','source_id','source_revision'])
  OR NEW.revocation_class NOT IN('LOST_OR_COMPROMISED','ACCESS_ENDED')
  THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device history immutable';END IF;
 IF NEW.source_type='COMMAND' THEN
  IF NEW.source_revision<>1 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.source_id AND action='idn.device.revoked' AND target_type='device' AND target_id=NEW.id AND reason_code='revoked')
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device command source unavailable';END IF;
 ELSIF NEW.source_type='MEMBERSHIP' THEN
  IF NEW.source_id<>NEW.membership_id OR NEW.revocation_class<>'ACCESS_ENDED' OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.membership_id AND state='ENDED' AND revision=NEW.source_revision)
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device membership source unavailable';END IF;
 ELSE RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device revocation source unavailable';END IF;
 NEW.revoked_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_device_enrollment_guard BEFORE INSERT OR UPDATE ON org_device_enrollments FOR EACH ROW EXECUTE FUNCTION org_device_enrollment_guard();

CREATE FUNCTION org_device_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE e public.org_device_enrollments%ROWTYPE;
BEGIN
 SELECT * INTO e FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.device_id;
 IF NOT FOUND OR NEW.result-ARRAY['id','membership_id','label','state','revision','revocation_class','enrolled_at','last_sync_at','revoked_at','source_type','source_id','source_revision']<>'{}'::jsonb
  OR NOT(NEW.result ?& ARRAY['id','membership_id','label','state','revision','revocation_class','enrolled_at','last_sync_at','revoked_at','source_type','source_id','source_revision'])
  OR NEW.result->>'id' IS DISTINCT FROM e.id::text OR NEW.result->>'membership_id' IS DISTINCT FROM e.membership_id::text
  OR NEW.result->>'label' IS DISTINCT FROM e.label OR NEW.result->>'state' IS DISTINCT FROM e.state OR NEW.result->>'revision' IS DISTINCT FROM e.revision::text
  OR NEW.result->>'revocation_class' IS DISTINCT FROM e.revocation_class OR(NEW.result->>'enrolled_at')::timestamptz IS DISTINCT FROM e.enrolled_at
  OR NEW.result->'last_sync_at' IS DISTINCT FROM 'null'::jsonb OR(NEW.result->>'revoked_at')::timestamptz IS DISTINCT FROM e.revoked_at
  OR NEW.result->>'source_type' IS DISTINCT FROM e.source_type OR NEW.result->>'source_id' IS DISTINCT FROM COALESCE(e.source_id::text,'') OR NEW.result->>'source_revision' IS DISTINCT FROM e.source_revision::text
  OR NOT EXISTS(SELECT 1 FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id AND actor_id=NEW.actor_id AND target_type='device' AND target_id=NEW.device_id AND request_hash=NEW.request_hash
   AND action=CASE NEW.kind WHEN 'ENROLL' THEN 'idn.device.enrolled' ELSE 'idn.device.revoked' END AND reason_code=CASE NEW.kind WHEN 'ENROLL' THEN 'enrolled' ELSE 'revoked' END)
  THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device acknowledgement source unavailable';END IF;
 IF NEW.kind='ENROLL' THEN
  IF e.state<>'ENROLLED' OR e.revision<>1 OR e.created_operation_id<>NEW.operation_id OR NEW.actor_key_id IS DISTINCT FROM e.key_id
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=e.membership_id AND account_id=NEW.actor_id AND state='ACTIVE')
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device enrollment acknowledgement unavailable';END IF;
 ELSIF e.state<>'REVOKED' OR e.source_type<>'COMMAND' OR e.source_id<>NEW.operation_id OR e.source_revision<>1 THEN
  RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device revocation acknowledgement unavailable';
 END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_device_command_guard BEFORE INSERT ON org_device_commands FOR EACH ROW EXECUTE FUNCTION org_device_command_guard();
CREATE FUNCTION org_device_command_source_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_commands WHERE org_id=NEW.org_id AND operation_id=NEW.created_operation_id AND device_id=NEW.id AND kind='ENROLL' AND actor_key_id=NEW.key_id)
   THEN RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device enrollment command missing';END IF;
 ELSIF NEW.source_type='COMMAND' AND NOT EXISTS(SELECT 1 FROM public.org_device_commands WHERE org_id=NEW.org_id AND operation_id=NEW.source_id AND device_id=NEW.id AND kind='REVOKE') THEN
  RAISE EXCEPTION USING ERRCODE='P9005',MESSAGE='device revocation command missing';
 END IF;
 RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER org_device_command_source_guard AFTER INSERT OR UPDATE ON org_device_enrollments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_device_command_source_guard();
CREATE FUNCTION org_device_membership_ended() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 UPDATE public.org_device_enrollments SET state='REVOKED',revision=revision+1,revocation_class='ACCESS_ENDED',source_type='MEMBERSHIP',source_id=NEW.id,source_revision=NEW.revision
 WHERE org_id=NEW.org_id AND membership_id=NEW.id AND state='ENROLLED';RETURN NEW;
END $$;
CREATE TRIGGER org_device_membership_ended AFTER UPDATE OF state ON org_memberships FOR EACH ROW WHEN(NEW.state='ENDED' AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_device_membership_ended();
