-- EMP-IDN-009: immutable manager suspension/reactivation acknowledgements.
-- Existing membership/role periods and source projections are not rewritten.
CREATE TABLE org_membership_access_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),membership_id UUID NOT NULL,kind TEXT NOT NULL CHECK(kind IN('SUSPEND','REACTIVATE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 280 AND reason !~ '[[:cntrl:]]'),
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=8192),occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(occurred_at)),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE org_membership_access_notices(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,membership_id UUID NOT NULL,revision BIGINT NOT NULL CHECK(revision>1),
 kind TEXT NOT NULL CHECK(kind IN('SUSPEND','REACTIVATE')),occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,operation_id) REFERENCES org_membership_access_commands(org_id,operation_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);
ALTER TABLE org_membership_access_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_membership_access_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_membership_access_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_membership_access_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_membership_access_command_scope ON org_membership_access_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_membership_access_notice_scope ON org_membership_access_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_membership_access_commands,org_membership_access_notices FROM PUBLIC;
CREATE TRIGGER org_membership_access_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_membership_access_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_membership_access_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_membership_access_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_membership_access_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF NEW.result-ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','previous_suspension_origin']<>'{}'::jsonb
  OR NOT(NEW.result ?& ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','previous_suspension_origin'])
  OR NOT EXISTS(SELECT 1 FROM public.org_memberships m WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND m.revision>1
   AND NEW.result->>'membership_id'=m.id::text AND NEW.result->>'employee_id'=m.employee_id::text AND NEW.result->>'access_model'=m.access_model
   AND NEW.result->>'state'=m.state AND NEW.result->>'revision'=m.revision::text AND NEW.result->>'suspension_origin'=m.suspension_origin AND NEW.result->>'end_reason'=m.end_reason
   AND((NEW.kind='SUSPEND' AND m.state='SUSPENDED' AND m.suspension_origin='MANAGER' AND NEW.result->>'previous_suspension_origin'='')
    OR(NEW.kind='REACTIVATE' AND m.state='ACTIVE' AND NEW.result->>'previous_suspension_origin' IN('MANAGER','EMPLOYEE_INACTIVE'))))
  OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_id=NEW.membership_id AND a.target_type='membership' AND a.request_hash=NEW.request_hash AND a.reason_code=CASE NEW.kind WHEN 'SUSPEND' THEN 'suspended' ELSE 'reactivated' END AND a.action=CASE NEW.kind WHEN 'SUSPEND' THEN 'idn.membership.suspended' ELSE 'idn.membership.reactivated' END)
 THEN RAISE EXCEPTION USING ERRCODE='P9001',MESSAGE='membership command source unavailable';END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_membership_access_command_guard BEFORE INSERT ON org_membership_access_commands FOR EACH ROW EXECUTE FUNCTION org_membership_access_command_guard();
CREATE FUNCTION org_membership_access_notice_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_membership_access_commands c WHERE c.org_id=NEW.org_id AND c.operation_id=NEW.operation_id AND c.membership_id=NEW.membership_id AND c.kind=NEW.kind AND c.result->>'revision'=NEW.revision::text AND c.occurred_at=NEW.occurred_at) THEN RAISE EXCEPTION USING ERRCODE='P9001',MESSAGE='membership notice source unavailable';END IF;RETURN NEW;
END $$;
CREATE TRIGGER org_membership_access_notice_guard BEFORE INSERT ON org_membership_access_notices FOR EACH ROW EXECUTE FUNCTION org_membership_access_notice_guard();
