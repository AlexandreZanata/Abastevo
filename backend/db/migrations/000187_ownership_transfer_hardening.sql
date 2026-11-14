-- PR203 review: forward-only ownership command/source hardening.
-- Existing 185 rows retain their actual values and empty source metadata.
ALTER TABLE org_ownership_transfers ADD COLUMN reduction_cause TEXT NOT NULL DEFAULT ''
 CHECK(reduction_cause IN('','MEMBERSHIP_REDUCED','ORGANIZATION_ARCHIVED','ORGANIZATION_CLOSED'));
ALTER TABLE org_ownership_transfers ADD COLUMN source_membership_id UUID;
ALTER TABLE org_ownership_transfers ADD COLUMN source_membership_revision BIGINT;
ALTER TABLE org_ownership_transfers ADD CONSTRAINT org_ownership_reduction_source CHECK(
 (reduction_cause='MEMBERSHIP_REDUCED' AND state='CANCELLED' AND source_membership_id IS NOT NULL AND source_membership_revision>1)
 OR(reduction_cause IN('ORGANIZATION_ARCHIVED','ORGANIZATION_CLOSED') AND state='CANCELLED' AND source_membership_id IS NULL AND source_membership_revision IS NULL)
 OR(reduction_cause='' AND source_membership_id IS NULL AND source_membership_revision IS NULL));

CREATE TABLE org_ownership_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),kind TEXT NOT NULL CHECK(kind IN('PROPOSE','ACCEPT','CANCEL')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),transfer_id UUID NOT NULL,result JSONB NOT NULL,
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,transfer_id) REFERENCES org_ownership_transfers(org_id,transfer_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(actor_account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=16384)
);
CREATE TABLE org_ownership_notices(
 org_id UUID NOT NULL,transfer_id UUID NOT NULL,kind TEXT NOT NULL CHECK(kind IN('PROPOSED','ACCEPTED','CANCELLED','DECLINED','EXPIRED')),
 audience TEXT NOT NULL DEFAULT 'PARTICIPANTS' CHECK(audience='PARTICIPANTS'),
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,transfer_id,kind),FOREIGN KEY(org_id,transfer_id) REFERENCES org_ownership_transfers(org_id,transfer_id)
);
ALTER TABLE org_ownership_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_ownership_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_ownership_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_ownership_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_ownership_command_scope ON org_ownership_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_ownership_notice_scope ON org_ownership_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_ownership_commands,org_ownership_notices FROM PUBLIC;
CREATE TRIGGER org_ownership_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_ownership_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_ownership_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_ownership_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

-- Invoker helper; callers hold account locks before the root. No account lock
-- is acquired here after the root, preserving admission/erasure lock order.
CREATE FUNCTION org_ownership_personal_member(selected_org UUID,selected_member UUID) RETURNS BOOLEAN
 LANGUAGE sql VOLATILE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_memberships m
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
 JOIN public.accounts a ON a.id=m.account_id
 WHERE m.org_id=selected_org AND m.id=selected_member AND m.state='ACTIVE' AND m.access_model='EXISTING_ACCOUNT'
 AND e.state='ACTIVE' AND a.status='active' AND a.enterprise_managed_org_id IS NULL
 AND NOT EXISTS(SELECT 1 FROM public.payment_billing_exits x WHERE x.account_id=a.id AND x.reason='ACCOUNT_DELETION'))
$$;
CREATE FUNCTION org_ownership_current_owner(selected_org UUID,selected_member UUID) RETURNS BOOLEAN
 LANGUAGE sql VOLATILE SET search_path=pg_catalog AS $$
 SELECT public.org_ownership_personal_member(selected_org,selected_member) AND EXISTS(
 SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=selected_org AND r.membership_id=selected_member
 AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=selected_org AND r.ended_at IS NULL
 AND r.valid_from<=clock_timestamp() AND(r.valid_to IS NULL OR r.valid_to>clock_timestamp()))
$$;
CREATE OR REPLACE FUNCTION org_ownership_transfer_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;at TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND OR NOT root_ready THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership source unavailable';END IF;
 at=clock_timestamp();
 IF TG_OP='INSERT' THEN
  IF root_state<>'ACTIVE' OR NEW.state IS DISTINCT FROM 'PENDING' OR NEW.accepted_at IS NOT NULL OR NEW.cancelled_at IS NOT NULL OR NEW.declined_at IS NOT NULL
   OR NEW.reduction_cause<>'' OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL
   OR NEW.initiator_membership_id=NEW.target_membership_id
   OR NOT public.org_ownership_current_owner(NEW.org_id,NEW.initiator_membership_id)
   OR NOT public.org_ownership_personal_member(NEW.org_id,NEW.target_membership_id)
   OR public.org_ownership_current_owner(NEW.org_id,NEW.target_membership_id) THEN RAISE EXCEPTION USING ERRCODE='P8701',MESSAGE='invalid ownership proposal';END IF;
  NEW.created_at=at;NEW.expires_at=at+interval '168 hours';RETURN NEW;
 END IF;
 IF OLD.state<>'PENDING' OR NEW.state NOT IN('ACCEPTED','CANCELLED','DECLINED','EXPIRED')
  OR ROW(NEW.org_id,NEW.transfer_id,NEW.initiator_membership_id,NEW.target_membership_id,NEW.step_down_to_admin,NEW.expires_at,NEW.created_at,NEW.operation_id)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.transfer_id,OLD.initiator_membership_id,OLD.target_membership_id,OLD.step_down_to_admin,OLD.expires_at,OLD.created_at,OLD.operation_id) THEN RAISE EXCEPTION USING ERRCODE='P8703',MESSAGE='ownership history is immutable';END IF;
 NEW.accepted_at=NULL;NEW.cancelled_at=NULL;NEW.declined_at=NULL;
 IF NEW.reduction_cause='MEMBERSHIP_REDUCED' THEN
  IF NEW.state<>'CANCELLED' OR NEW.source_membership_id IS NULL OR NEW.source_membership_revision IS NULL
   OR NEW.source_membership_id NOT IN(OLD.initiator_membership_id,OLD.target_membership_id)
   OR NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.source_membership_id AND revision=NEW.source_membership_revision AND revision>1 AND state IN('SUSPENDED','ENDED')) THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership reduction source unavailable';END IF;
 ELSIF NEW.reduction_cause IN('ORGANIZATION_ARCHIVED','ORGANIZATION_CLOSED') THEN
  IF NEW.state<>'CANCELLED' OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL
   OR(root_state<>'ARCHIVED' AND NEW.reduction_cause='ORGANIZATION_ARCHIVED') OR(root_state<>'CLOSED' AND NEW.reduction_cause='ORGANIZATION_CLOSED') THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership reduction source unavailable';END IF;
 ELSIF NEW.reduction_cause<>'' OR NEW.source_membership_id IS NOT NULL OR NEW.source_membership_revision IS NOT NULL THEN
  RAISE EXCEPTION USING ERRCODE='P8701',MESSAGE='invalid ownership source';
 END IF;
 IF NEW.state='ACCEPTED' THEN
  IF root_state<>'ACTIVE' OR OLD.expires_at<=at OR NOT isfinite(OLD.created_at) OR OLD.created_at>at OR NOT isfinite(OLD.expires_at)
   OR NOT public.org_ownership_personal_member(NEW.org_id,NEW.initiator_membership_id)
   OR NOT public.org_ownership_current_owner(NEW.org_id,NEW.target_membership_id)
   OR NEW.reduction_cause<>'' THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership acceptance source unavailable';END IF;
  IF NEW.step_down_to_admin THEN
   IF EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=NEW.org_id AND r.membership_id=NEW.initiator_membership_id AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=NEW.org_id AND r.ended_at IS NULL AND(r.valid_to IS NULL OR r.valid_to>at))
    OR NOT EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=NEW.org_id AND r.membership_id=NEW.initiator_membership_id AND r.role='OWNER' AND r.scope_type='ORG' AND r.scope_target=NEW.org_id AND r.valid_from<=OLD.created_at AND(r.valid_to IS NULL OR r.valid_to>OLD.created_at) AND r.ended_at>=OLD.created_at)
    OR NOT EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=NEW.org_id AND r.membership_id=NEW.initiator_membership_id AND r.role='ADMIN' AND r.scope_type='ORG' AND r.scope_target=NEW.org_id AND r.ended_at IS NULL AND r.valid_from<=at AND(r.valid_to IS NULL OR r.valid_to>at)) THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership step-down source unavailable';END IF;
  ELSIF NOT public.org_ownership_current_owner(NEW.org_id,NEW.initiator_membership_id) THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership initiator unavailable';END IF;
  NEW.accepted_at=at;
 ELSIF NEW.state='EXPIRED' THEN
  IF OLD.expires_at>at OR NOT isfinite(OLD.expires_at) OR NEW.reduction_cause<>'' THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership expiration source unavailable';END IF;
 ELSIF NEW.state='CANCELLED' THEN NEW.cancelled_at=at;
 ELSE
  IF NEW.reduction_cause<>'' THEN RAISE EXCEPTION USING ERRCODE='P8701',MESSAGE='invalid ownership decline source';END IF;NEW.declined_at=at;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_ownership_transfer_consume_membership() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_ownership_transfers SET state='CANCELLED',cancelled_at=clock_timestamp(),reduction_cause='MEMBERSHIP_REDUCED',source_membership_id=NEW.id,source_membership_revision=NEW.revision
 WHERE org_id=NEW.org_id AND(initiator_membership_id=NEW.id OR target_membership_id=NEW.id) AND state='PENDING';RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_ownership_transfer_consume_organization() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_ownership_transfers SET state='CANCELLED',cancelled_at=clock_timestamp(),reduction_cause=CASE WHEN NEW.state='CLOSED' THEN 'ORGANIZATION_CLOSED' ELSE 'ORGANIZATION_ARCHIVED' END,source_membership_id=NULL,source_membership_revision=NULL
 WHERE org_id=NEW.org_id AND state='PENDING';RETURN NEW;
END $$;
CREATE FUNCTION org_ownership_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR NOT EXISTS(SELECT 1 FROM public.org_ownership_transfers t WHERE t.org_id=NEW.org_id AND t.transfer_id=NEW.transfer_id AND((NEW.kind='PROPOSED' AND t.state='PENDING') OR(NEW.kind<>'PROPOSED' AND t.state=NEW.kind))) THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_ownership_notice_guard BEFORE INSERT ON org_ownership_notices FOR EACH ROW EXECUTE FUNCTION org_ownership_notice_guard();
CREATE FUNCTION org_ownership_transfer_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 INSERT INTO public.org_ownership_notices(org_id,transfer_id,kind) VALUES(NEW.org_id,NEW.transfer_id,CASE WHEN NEW.state='PENDING' THEN 'PROPOSED' ELSE NEW.state END);RETURN NEW;
END $$;
CREATE TRIGGER org_ownership_transfer_project AFTER INSERT OR UPDATE OF state ON org_ownership_transfers FOR EACH ROW EXECUTE FUNCTION org_ownership_transfer_project();
CREATE FUNCTION org_ownership_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR NOT EXISTS(
 SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_id=NEW.transfer_id AND a.target_type='ownership_transfer' AND a.request_hash=NEW.request_hash
 AND((NEW.kind='PROPOSE' AND a.action='org.ownership_transfer.proposed') OR(NEW.kind='ACCEPT' AND a.action='org.ownership_transfer.accepted') OR(NEW.kind='CANCEL' AND a.action IN('org.ownership_transfer.cancelled','org.ownership_transfer.declined')))) THEN RAISE EXCEPTION USING ERRCODE='P8702',MESSAGE='ownership command source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_ownership_command_guard BEFORE INSERT ON org_ownership_commands FOR EACH ROW EXECUTE FUNCTION org_ownership_command_guard();
DO $$ DECLARE owner_name TEXT;BEGIN
 SELECT pg_catalog.pg_get_userbyid(relowner) INTO owner_name FROM pg_catalog.pg_class WHERE oid='public.org_memberships'::regclass;
 EXECUTE pg_catalog.format('GRANT SELECT,UPDATE ON public.org_ownership_transfers TO %I',owner_name);
 EXECUTE pg_catalog.format('GRANT SELECT,INSERT ON public.org_ownership_notices TO %I',owner_name);
END $$;
