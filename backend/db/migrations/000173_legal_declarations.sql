-- Accept the plan's bounded snake_case vocabulary; exact registrations remain
-- in source. Existing historical rows are unchanged and satisfy the new checks.
ALTER TABLE org_audit_events DROP CONSTRAINT org_audit_events_action_check,
 ADD CONSTRAINT org_audit_events_action_check CHECK(length(action) BETWEEN 1 AND 64 AND action ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
 DROP CONSTRAINT org_audit_events_target_type_check,
 ADD CONSTRAINT org_audit_events_target_type_check CHECK(length(target_type) BETWEEN 1 AND 64 AND target_type ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
 DROP CONSTRAINT org_audit_events_reason_code_check,
 ADD CONSTRAINT org_audit_events_reason_code_check CHECK(reason_code='' OR (length(reason_code)<=64 AND reason_code ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'));
ALTER TABLE org_event_intents DROP CONSTRAINT org_event_intents_consumer_check,
 ADD CONSTRAINT org_event_intents_consumer_check CHECK(length(consumer)<=64 AND consumer ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
 DROP CONSTRAINT org_event_intents_event_name_check,
 ADD CONSTRAINT org_event_intents_event_name_check CHECK(length(event_name)<=64 AND event_name ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$'),
 DROP CONSTRAINT org_event_intents_target_type_check,
 ADD CONSTRAINT org_event_intents_target_type_check CHECK(length(target_type)<=64 AND target_type ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$');

-- EMP-ORG-004: private declarations, not registry verification or access.
CREATE FUNCTION org_legal_cpf_valid(value TEXT) RETURNS BOOLEAN
 LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
DECLARE ending INTEGER;i INTEGER;total INTEGER;digit INTEGER;remainder INTEGER;
BEGIN
 IF value IS NULL OR value !~ '^[0-9]{11}$' OR value=repeat(substr(value,1,1),11) THEN RETURN false; END IF;
 FOR ending IN 9..10 LOOP
  total=0;
  FOR i IN 1..ending LOOP total=total+(ascii(substr(value,i,1))-48)*(ending+2-i); END LOOP;
  remainder=total%11;digit=CASE WHEN remainder<2 THEN 0 ELSE 11-remainder END;
  IF ascii(substr(value,ending+1,1))-48<>digit THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
END $$;
CREATE FUNCTION org_legal_cnpj_valid(value TEXT) RETURNS BOOLEAN
 LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
DECLARE ending INTEGER;i INTEGER;total INTEGER;digit INTEGER;weight INTEGER;remainder INTEGER;
BEGIN
 IF value IS NULL OR value !~ '^[A-Z0-9]{12}[0-9]{2}$' THEN RETURN false; END IF;
 FOR ending IN 12..13 LOOP
  total=0;weight=2;
  FOR i IN REVERSE ending..1 LOOP
   total=total+(ascii(substr(value,i,1))-48)*weight;weight=CASE WHEN weight=9 THEN 2 ELSE weight+1 END;
  END LOOP;
  remainder=total%11;digit=CASE WHEN remainder<2 THEN 0 ELSE 11-remainder END;
  IF ascii(substr(value,ending+1,1))-48<>digit THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
END $$;
CREATE FUNCTION org_legal_name_valid(value TEXT,required BOOLEAN) RETURNS BOOLEAN
 LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT value IS NOT NULL AND length(value)<=120 AND value=btrim(value)
  AND (NOT required OR length(value)>0) AND value !~ '[[:cntrl:]]' AND value !~ U&'[\202a-\202e\2066-\2069]'
$$;

CREATE TABLE org_legal_entities(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL,
 kind TEXT NOT NULL,legal_name TEXT NOT NULL,identifier TEXT NOT NULL,
 purpose TEXT NOT NULL DEFAULT '',state TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1,
 verification_history BOOLEAN NOT NULL DEFAULT false,
 withdrawn_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id)
);
-- Only one current declaration. Revoked history retains its old identity while
-- a later explicit declaration uses a new ID (organization specification §6).
CREATE UNIQUE INDEX org_legal_current_identifier ON org_legal_entities(org_id,kind,identifier) WHERE state<>'REVOKED';
ALTER TABLE org_legal_entities ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_legal_entities FORCE ROW LEVEL SECURITY;
CREATE POLICY org_legal_entity_scope ON org_legal_entities
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_legal_entities FROM PUBLIC;

CREATE TABLE org_establishments(
 org_id UUID NOT NULL,id UUID NOT NULL,legal_entity_id UUID NOT NULL,
 cnpj TEXT NOT NULL,declared_role TEXT NOT NULL DEFAULT 'NOT_INFORMED',
 trade_name TEXT NOT NULL DEFAULT '',municipality_code TEXT NOT NULL DEFAULT '',
 revision BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),FOREIGN KEY(org_id,legal_entity_id) REFERENCES org_legal_entities(org_id,id) ON DELETE CASCADE,
 UNIQUE(org_id,legal_entity_id,cnpj)
);
ALTER TABLE org_establishments ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_establishments FORCE ROW LEVEL SECURITY;
CREATE POLICY org_establishment_scope ON org_establishments
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_establishments FROM PUBLIC;

CREATE TABLE org_legal_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES accounts(id),kind TEXT NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 legal_entity_id UUID NOT NULL,establishment_id UUID,
 result_state TEXT NOT NULL CHECK(result_state IN ('DECLARED','REVOKED','REMOVED')),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid AND legal_entity_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 CHECK(kind IN ('DECLARE','ADD_ESTABLISHMENT','CHANGE_ESTABLISHMENT','WITHDRAW')),
 CHECK((kind='DECLARE' AND result_state='DECLARED' AND result_revision=1)
  OR (kind='ADD_ESTABLISHMENT' AND establishment_id IS NOT NULL AND result_state='DECLARED' AND result_revision=1)
  OR (kind='CHANGE_ESTABLISHMENT' AND establishment_id IS NOT NULL AND result_state='DECLARED' AND result_revision>1)
  OR (kind='WITHDRAW' AND establishment_id IS NULL AND result_state IN ('REVOKED','REMOVED')))
);
-- No FK to removable declaration content: acknowledgements retain only IDs,
-- bounded outcome and a scoped operation hash, never legal names or CPF.
CREATE UNIQUE INDEX org_legal_creation_identity ON org_legal_commands(org_id,legal_entity_id) WHERE kind='DECLARE';
CREATE UNIQUE INDEX org_establishment_creation_identity ON org_legal_commands(org_id,establishment_id) WHERE kind IN ('DECLARE','ADD_ESTABLISHMENT') AND establishment_id IS NOT NULL;
ALTER TABLE org_legal_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_legal_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_legal_command_scope ON org_legal_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_legal_commands FROM PUBLIC;
CREATE TRIGGER org_legal_command_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_legal_commands
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_legal_command_time BEFORE INSERT ON org_legal_commands
 FOR EACH ROW EXECUTE FUNCTION org_onboarding_created_time();

CREATE FUNCTION org_legal_entity_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;root_state TEXT;root_ready BOOLEAN;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT state,has_snapshot INTO root_state,root_ready FROM public.organizations WHERE org_id=selected_org FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NOT root_ready OR root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept declaration changes'; END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.verification_history THEN RAISE EXCEPTION USING ERRCODE='P7602',MESSAGE='declaration history must be retained'; END IF;
  RETURN OLD;
 END IF;
 -- Explicit static trigger refusals avoid the full failing-row/identifier
 -- DETAIL emitted by ordinary CHECK/unique errors, including CPF duplicates.
 IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid OR NEW.kind IS NULL
  OR NOT COALESCE(public.org_legal_name_valid(NEW.legal_name,true),false)
  OR NEW.state IS NULL OR NEW.state NOT IN ('DECLARED','REVOKED') OR NEW.revision IS NULL OR NEW.revision<1
  OR NEW.verification_history IS NULL OR NEW.identifier IS NULL OR NEW.purpose IS NULL
  OR NOT COALESCE((NEW.kind='COMPANY' AND NEW.identifier ~ '^[A-Z0-9]{8}$' AND NEW.purpose='')
   OR (NEW.kind='NATURAL_PERSON' AND public.org_legal_cpf_valid(NEW.identifier) AND NEW.purpose='VERIFICATION_AND_AGREEMENTS'),false) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid legal declaration';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'DECLARED' OR NEW.revision<>1 OR NEW.verification_history OR NEW.withdrawn_at IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial legal declaration';
  END IF;
  IF EXISTS(SELECT 1 FROM public.org_legal_commands WHERE org_id=NEW.org_id AND legal_entity_id=NEW.id AND kind='DECLARE') THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='declaration identity cannot be reused';
  END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF (NEW.org_id,NEW.id,NEW.kind,NEW.identifier) IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.kind,OLD.identifier)
   OR NEW.purpose<>OLD.purpose OR NEW.verification_history IS DISTINCT FROM OLD.verification_history THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='legal identity is immutable';
  END IF;
  IF OLD.state='REVOKED' OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid declaration revision';
  END IF;
  NEW.withdrawn_at=CASE WHEN NEW.state='REVOKED' THEN clock_timestamp() ELSE NULL END;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF NEW.state<>'REVOKED' AND EXISTS(SELECT 1 FROM public.org_legal_entities WHERE org_id=NEW.org_id AND kind=NEW.kind AND identifier=NEW.identifier AND id<>NEW.id AND state<>'REVOKED') THEN
  RAISE EXCEPTION USING ERRCODE='P7601',MESSAGE='duplicate legal declaration';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_legal_entity_revision BEFORE INSERT OR UPDATE OR DELETE ON org_legal_entities
 FOR EACH ROW EXECUTE FUNCTION org_legal_entity_guard();
CREATE TRIGGER org_legal_entity_no_truncate BEFORE TRUNCATE ON org_legal_entities
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_establishment_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;parent UUID;root_state TEXT;parent_kind TEXT;parent_identifier TEXT;parent_state TEXT;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 parent=CASE WHEN TG_OP='DELETE' THEN OLD.legal_entity_id ELSE NEW.legal_entity_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied'; END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF root_state NOT IN ('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept establishment changes'; END IF;
 SELECT kind,identifier,state INTO parent_kind,parent_identifier,parent_state FROM public.org_legal_entities WHERE org_id=selected_org AND id=parent FOR UPDATE;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 IF NOT FOUND OR parent_kind<>'COMPANY' OR parent_state<>'DECLARED' THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.id IS NULL OR NEW.id='00000000-0000-0000-0000-000000000000'::uuid OR NEW.id=NEW.legal_entity_id
  OR NOT public.org_legal_cnpj_valid(NEW.cnpj) OR substr(NEW.cnpj,1,8)<>parent_identifier
  OR NEW.declared_role IS NULL OR NEW.declared_role NOT IN ('HEAD_OFFICE','BRANCH','NOT_INFORMED')
  OR NOT COALESCE(public.org_legal_name_valid(NEW.trade_name,false),false)
  OR NEW.municipality_code IS NULL OR (NEW.municipality_code<>'' AND NEW.municipality_code !~ '^[1-9][0-9]{6}$')
  OR NEW.revision IS NULL OR NEW.revision<1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid establishment declaration'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial establishment'; END IF;
  IF EXISTS(SELECT 1 FROM public.org_legal_commands WHERE org_id=NEW.org_id AND establishment_id=NEW.id AND kind IN ('DECLARE','ADD_ESTABLISHMENT')) THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='establishment identity cannot be reused';
  END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF (NEW.org_id,NEW.id,NEW.legal_entity_id,NEW.cnpj) IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.legal_entity_id,OLD.cnpj) THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='establishment identity is immutable'; END IF;
  IF OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid establishment revision'; END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF EXISTS(SELECT 1 FROM public.org_establishments WHERE org_id=NEW.org_id AND legal_entity_id=NEW.legal_entity_id AND cnpj=NEW.cnpj AND id<>NEW.id) THEN
  RAISE EXCEPTION USING ERRCODE='P7601',MESSAGE='duplicate establishment declaration';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_establishment_revision BEFORE INSERT OR UPDATE OR DELETE ON org_establishments
 FOR EACH ROW EXECUTE FUNCTION org_establishment_guard();
CREATE TRIGGER org_establishment_no_truncate BEFORE TRUNCATE ON org_establishments
 FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_legal_company_establishment_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;parent UUID;prior_org TEXT;
BEGIN
 IF TG_TABLE_NAME='org_legal_entities' THEN selected_org=NEW.org_id;parent=NEW.id;
 ELSE selected_org=OLD.org_id;parent=OLD.legal_entity_id;END IF;
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',selected_org::text,true);
 IF EXISTS(SELECT 1 FROM public.org_legal_entities WHERE org_id=selected_org AND id=parent AND kind='COMPANY')
  AND NOT EXISTS(SELECT 1 FROM public.org_establishments WHERE org_id=selected_org AND legal_entity_id=parent) THEN
  RAISE EXCEPTION USING ERRCODE='P7603',MESSAGE='company declaration requires an establishment';
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_legal_company_establishment AFTER INSERT ON org_legal_entities
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='COMPANY') EXECUTE FUNCTION org_legal_company_establishment_guard();
CREATE CONSTRAINT TRIGGER org_establishment_company_presence AFTER DELETE ON org_establishments
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_legal_company_establishment_guard();
