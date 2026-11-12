-- EMP-ORG-005: retain unit identity, capture optional scoped metadata and
-- serialize configuration/assignment admission on the existing root row.
ALTER TABLE org_units ADD COLUMN establishment_id UUID,
 ADD COLUMN municipality_code TEXT NOT NULL DEFAULT '',
 ADD CONSTRAINT org_unit_establishment_fkey FOREIGN KEY(org_id,establishment_id)
  REFERENCES org_establishments(org_id,id);

CREATE TABLE org_unit_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES accounts(id),unit_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('CREATE','CHANGE','ARCHIVE','RESTORE')),
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 result_state TEXT NOT NULL CHECK(result_state IN ('ACTIVE','ARCHIVED')),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_unit_commands_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK((kind='CREATE' AND result_state='ACTIVE' AND result_revision=1)
  OR (kind IN ('CHANGE','RESTORE') AND result_revision>1 AND (kind<>'RESTORE' OR result_state='ACTIVE'))
  OR (kind='ARCHIVE' AND result_revision>1 AND result_state='ARCHIVED'))
);
ALTER TABLE org_unit_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_unit_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_unit_command_scope ON org_unit_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_unit_commands FROM PUBLIC;
CREATE TRIGGER org_unit_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_unit_commands
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_unit_command_time BEFORE INSERT ON org_unit_commands
 FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

-- Owning operational modules extend this exact source guard in their additive
-- migrations, with real current counts and root-serialized assignment writes.
-- There are currently no operational unit consumers. Immutable receipts do
-- not count as usage. Any unknown FK fails closed, including a future consumer
-- installed before its usage policy. This is not a configurable usage ledger.
CREATE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN
  RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
  AND NOT ((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
   OR (c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',0,'employees',0,'budgets',0,'operations',0);
END $$;

-- Future source-owned assignment triggers call this inside their transaction,
-- before admitting active work. Archive holds the same root lock until commit.
CREATE FUNCTION org_unit_admit(selected_org UUID,selected_unit UUID) RETURNS void
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state<>'ACTIVE' THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept unit work';
 END IF;
 PERFORM 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit AND state='ACTIVE';
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found'; END IF;
END $$;

CREATE OR REPLACE FUNCTION org_unit_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;root_ready BOOLEAN;usage JSONB;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=NEW.org_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept unit changes';
 END IF;
 IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid
  OR NEW.revision IS NULL OR NEW.revision<1 OR NEW.state IS NULL OR NEW.state NOT IN ('ACTIVE','ARCHIVED')
  OR NOT public.org_legal_name_valid(NEW.name,true) OR NOT public.org_legal_name_valid(NEW.code,true)
  OR NEW.municipality_code IS NULL OR (NEW.municipality_code<>'' AND NEW.municipality_code !~ '^[1-9][0-9]{6}$')
  OR NEW.time_zone IS NULL OR length(NEW.time_zone)>128 OR NEW.time_zone !~ '^[A-Za-z0-9_+/-]+$'
  OR (NEW.time_zone<>'UTC' AND position('/' IN NEW.time_zone)=0)
  OR NEW.time_zone ~ '^(posix/|right/|SystemV/)' OR NEW.time_zone ~ '(^/|/$|//)'
  OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.time_zone) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid unit profile';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>1 OR NEW.state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial unit'; END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid unit identity or revision';
  END IF;
  IF OLD.state='ACTIVE' AND NEW.state='ARCHIVED' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=OLD.org_id AND id<>OLD.id AND state='ACTIVE') THEN
    RAISE EXCEPTION USING ERRCODE='P7506',MESSAGE='last active unit protected';
   END IF;
   usage=public.org_unit_current_usage(OLD.org_id,OLD.id);
   IF usage IS NULL OR jsonb_typeof(usage)<>'object' THEN
    RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';
   END IF;
   IF NOT usage ?& ARRAY['assets','employees','budgets','operations'] OR (SELECT count(*) FROM jsonb_object_keys(usage))<>4
    OR jsonb_typeof(usage->'assets')<>'number' OR jsonb_typeof(usage->'employees')<>'number'
    OR jsonb_typeof(usage->'budgets')<>'number' OR jsonb_typeof(usage->'operations')<>'number'
    OR usage->>'assets' !~ '^[0-9]+$' OR usage->>'employees' !~ '^[0-9]+$'
    OR usage->>'budgets' !~ '^[0-9]+$' OR usage->>'operations' !~ '^[0-9]+$' THEN
    RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';
   END IF;
   IF (usage->>'assets')::bigint>0 OR (usage->>'employees')::bigint>0
    OR (usage->>'budgets')::bigint>0 OR (usage->>'operations')::bigint>0 THEN
    RAISE EXCEPTION USING ERRCODE='P7703',MESSAGE='structure currently in use';
   END IF;
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF NEW.establishment_id IS NOT NULL AND (TG_OP='INSERT' OR NEW.establishment_id IS DISTINCT FROM OLD.establishment_id) THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_establishments e JOIN public.org_legal_entities l ON (l.org_id,l.id)=(e.org_id,e.legal_entity_id)
   WHERE e.org_id=NEW.org_id AND e.id=NEW.establishment_id AND l.kind='COMPANY' AND l.state='DECLARED') THEN
   RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';
  END IF;
 END IF;
 IF NEW.state='ACTIVE' THEN
  IF EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id<>NEW.id AND state='ACTIVE' AND lower(name)=lower(NEW.name)) THEN
   RAISE EXCEPTION USING ERRCODE='P7701',MESSAGE='duplicate active unit name';
  END IF;
  IF EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id<>NEW.id AND state='ACTIVE' AND lower(code)=lower(NEW.code)) THEN
   RAISE EXCEPTION USING ERRCODE='P7702',MESSAGE='duplicate active unit code';
  END IF;
 END IF;
 RETURN NEW;
END $$;
