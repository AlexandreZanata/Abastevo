-- Tracking policy command (coordinator-assigned prerequisite of the
-- EMP-GPS-002 activation; B-BR-EMP-467/468, EDR-302/303). The insert-only
-- policy versions of 000240 stay unchanged; this adds only the immutable
-- command receipt that names the version an administrator wrote, its author
-- and its request digest, for replay and attribution. A version and its
-- receipt commit in one transaction; a receipt always names the newest
-- version, and an enabled version's notice was in force at its effective
-- instant.
CREATE TABLE org_tracking_policy_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL,
 actor_account_id UUID NOT NULL,
 actor_membership_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_tracking_policy_commands_revision_key UNIQUE(org_id,result_revision),
 CONSTRAINT org_tracking_policy_commands_version_fkey FOREIGN KEY(org_id,result_revision) REFERENCES org_tracking_policies(org_id,revision),
 CONSTRAINT org_tracking_policy_commands_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE FUNCTION org_tracking_policy_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p public.org_tracking_policies%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO p FROM public.org_tracking_policies WHERE org_id=NEW.org_id ORDER BY revision DESC LIMIT 1;
 IF NOT FOUND OR p.revision<>NEW.result_revision THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking policy command revision unavailable';END IF;
 IF p.policy<>'OFF' AND p.notice_in_force_at>p.effective_at THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking policy notice not in force';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.account_id=NEW.actor_account_id AND m.state='ACTIVE')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking policy command actor unavailable';END IF;
 NEW.created_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_policy_command_guard BEFORE INSERT ON org_tracking_policy_commands FOR EACH ROW EXECUTE FUNCTION org_tracking_policy_command_guard();

ALTER TABLE org_tracking_policy_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_tracking_policy_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_tracking_policy_commands_scope ON org_tracking_policy_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_tracking_policy_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_tracking_policy_commands FOR EACH STATEMENT EXECUTE FUNCTION org_tracking_keep_history();
REVOKE ALL ON org_tracking_policy_commands FROM PUBLIC;
