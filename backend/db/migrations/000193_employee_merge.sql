-- EMP-IDN-012. Preserve historical IDs, source pointers and actual team periods.
CREATE INDEX org_employee_merged_pointer ON org_employee_identities(org_id,merged_into) WHERE merged_into IS NOT NULL;
CREATE TABLE org_employee_merge_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),source_employee_id UUID NOT NULL,survivor_employee_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),review_digest TEXT NOT NULL CHECK(review_digest~'^[a-f0-9]{64}$'),
 reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 280 AND reason !~ '[[:cntrl:]]'),
 source_revision BIGINT NOT NULL CHECK(source_revision>1),survivor_revision BIGINT NOT NULL CHECK(survivor_revision>0),
 survivor_state TEXT NOT NULL CHECK(survivor_state IN('ACTIVE','INACTIVE','TERMINATED')),
 team_effects JSONB NOT NULL CHECK(jsonb_typeof(team_effects)='array' AND jsonb_array_length(team_effects)<=500 AND octet_length(team_effects::text)<=131072),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(occurred_at)),
 PRIMARY KEY(org_id,operation_id),UNIQUE(org_id,source_employee_id),CHECK(source_employee_id<>survivor_employee_id),
 FOREIGN KEY(org_id,source_employee_id) REFERENCES org_employee_identities(org_id,id),FOREIGN KEY(org_id,survivor_employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE org_employee_merge_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_employee_merge_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_employee_merge_scope ON org_employee_merge_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_employee_merge_commands FROM PUBLIC;
CREATE TRIGGER org_employee_merge_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_employee_merge_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_employee_merge_command_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE item JSONB;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_employee_identities a JOIN public.org_employee_identities b ON b.org_id=a.org_id WHERE a.org_id=NEW.org_id AND a.id=NEW.source_employee_id AND a.state='TERMINATED' AND a.merged_into=NEW.survivor_employee_id AND a.revision=NEW.source_revision AND b.id=NEW.survivor_employee_id AND b.merged_into IS NULL AND b.revision=NEW.survivor_revision AND b.state=NEW.survivor_state)
 OR EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND employee_id=NEW.source_employee_id AND state<>'ENDED')
 OR EXISTS(SELECT 1 FROM public.org_team_links WHERE org_id=NEW.org_id AND employee_id=NEW.source_employee_id AND ended_at IS NULL)
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id AND actor_id=NEW.actor_account_id AND action='idn.employee.merged' AND target_type='employee' AND target_id=NEW.source_employee_id AND reason_code='merged' AND request_hash=NEW.request_hash)
 THEN RAISE EXCEPTION USING ERRCODE='P9004',MESSAGE='employee merge source unavailable';END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.team_effects) LOOP
  IF item-ARRAY['team_id','source_link_id','survivor_link_id','disposition']<>'{}'::jsonb OR NOT(item ?& ARRAY['team_id','source_link_id','survivor_link_id','disposition']) OR NOT EXISTS(
   SELECT 1 FROM public.org_team_links a JOIN public.org_team_links b ON b.org_id=a.org_id AND b.team_id=a.team_id
   WHERE a.org_id=NEW.org_id AND a.id::text=item->>'source_link_id' AND a.team_id::text=item->>'team_id' AND a.employee_id=NEW.source_employee_id AND a.end_reason='REMOVED' AND a.closed_operation_id=NEW.operation_id AND a.ended_at IS NOT NULL
    AND b.id::text=item->>'survivor_link_id' AND b.employee_id=NEW.survivor_employee_id AND b.ended_at IS NULL
    AND((item->>'disposition'='CREATED' AND b.created_operation_id=NEW.operation_id) OR(item->>'disposition'='EXISTING' AND b.created_operation_id<>NEW.operation_id)))
  THEN RAISE EXCEPTION USING ERRCODE='P9004',MESSAGE='employee merge team source unavailable';END IF;
 END LOOP;
 IF (SELECT count(*) FROM public.org_team_links WHERE org_id=NEW.org_id AND employee_id=NEW.source_employee_id AND closed_operation_id=NEW.operation_id)<>jsonb_array_length(NEW.team_effects) THEN RAISE EXCEPTION USING ERRCODE='P9004',MESSAGE='employee merge team history unavailable';END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_employee_merge_command_guard BEFORE INSERT ON org_employee_merge_commands FOR EACH ROW EXECUTE FUNCTION org_employee_merge_command_guard();
CREATE OR REPLACE FUNCTION org_employee_identity_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ACTIVE' OR NEW.revision<>1 OR NEW.merged_into IS NOT NULL OR NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid employee identity';
  END IF;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.merged_into IS NOT NULL
   OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid employee revision or identity';
  END IF;
  IF (NEW.state=OLD.state AND NOT(NEW.state='TERMINATED' AND NEW.merged_into IS NOT NULL)) OR NOT((OLD.state='ACTIVE' AND NEW.state IN ('INACTIVE','TERMINATED'))
   OR (OLD.state='INACTIVE' AND NEW.state IN ('ACTIVE','TERMINATED')) OR (OLD.state='TERMINATED' AND NEW.state='ACTIVE') OR (OLD.state='TERMINATED' AND NEW.state='TERMINATED' AND NEW.merged_into IS NOT NULL)) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee transition not allowed';
  END IF;
  IF NEW.merged_into IS NOT NULL THEN
   IF NEW.state<>'TERMINATED' OR NEW.merged_into=NEW.id OR NOT root_ready OR root_state NOT IN('PENDING_SETUP','ACTIVE')
    OR NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.merged_into AND merged_into IS NULL)
    OR EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND employee_id=NEW.id AND state<>'ENDED')
    OR EXISTS(SELECT 1 FROM public.org_team_links WHERE org_id=NEW.org_id AND employee_id=NEW.id AND ended_at IS NULL)
   THEN RAISE EXCEPTION USING ERRCODE='P9004',MESSAGE='employee merge conflict';END IF;
   IF (WITH RECURSIVE ancestors(id,depth) AS(SELECT NEW.id,0 UNION ALL SELECT e.id,a.depth+1 FROM public.org_employee_identities e JOIN ancestors a ON e.merged_into=a.id WHERE e.org_id=NEW.org_id AND a.depth<64) SELECT COALESCE(max(depth),0) FROM ancestors)>=64 THEN RAISE EXCEPTION USING ERRCODE='P9004',MESSAGE='employee merge depth exceeded';END IF;
  END IF;
  IF EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND employee_id=NEW.id
   AND ((NEW.state='INACTIVE' AND state IN ('INVITED','ACTIVE')) OR (NEW.state='TERMINATED' AND state<>'ENDED'))) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='membership effects must precede employee change';
  END IF;
 END IF;
 RETURN NEW;
END $$;
