-- Proposed additive migration 000186, owned by root.
CREATE TABLE org_role_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL REFERENCES accounts(id),kind TEXT NOT NULL CHECK(kind IN('ASSIGN','END','CHANGE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),result JSONB NOT NULL CHECK(jsonb_typeof(result)='object'),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_role_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_role_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_role_command_scope ON org_role_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_role_commands FROM PUBLIC;
CREATE TRIGGER org_role_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_role_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
