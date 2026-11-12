-- EMP-IDN-011. Actual employee rehire acknowledgements, no access restoration.
CREATE TABLE org_employee_rehire_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),employee_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 previous_revision BIGINT NOT NULL CHECK(previous_revision BETWEEN 1 AND 9223372036854775806),
 result_revision BIGINT NOT NULL CHECK(result_revision=previous_revision+1),
 result_state TEXT NOT NULL CHECK(result_state='ACTIVE'),
 reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 280 AND reason !~ '[[:cntrl:]]'),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(occurred_at)),
 PRIMARY KEY(org_id,operation_id),UNIQUE(org_id,employee_id,result_revision),
 FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_employee_rehire_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_employee_rehire_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_rehire_scope ON org_employee_rehire_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_employee_rehire_commands FROM PUBLIC;
CREATE TRIGGER org_employee_rehire_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_employee_rehire_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_employee_rehire_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_employee_identities e WHERE e.org_id=NEW.org_id AND e.id=NEW.employee_id AND e.state='ACTIVE' AND e.merged_into IS NULL AND e.revision=NEW.result_revision)
 OR EXISTS(SELECT 1 FROM public.org_memberships m WHERE m.org_id=NEW.org_id AND m.employee_id=NEW.employee_id AND m.state<>'ENDED')
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_type='employee' AND a.target_id=NEW.employee_id AND a.action='idn.employee.rehired' AND a.reason_code='rehired' AND a.request_hash=NEW.request_hash)
 THEN RAISE EXCEPTION USING ERRCODE='P9003',MESSAGE='rehire source unavailable';END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_employee_rehire_guard BEFORE INSERT ON org_employee_rehire_commands FOR EACH ROW EXECUTE FUNCTION org_employee_rehire_guard();
