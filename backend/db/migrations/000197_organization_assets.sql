-- EMP-FLEET-002. Owned private assets; no runtime grants or meter provider.
-- This immutable SQL projection belongs to fuel-products-v1, not a second fleet catalog.
CREATE TABLE org_fuel_products_v1(
 code TEXT PRIMARY KEY,unit TEXT NOT NULL,storage_unit TEXT NOT NULL,category TEXT NOT NULL,family TEXT NOT NULL,
 version TEXT NOT NULL CHECK(version='fuel-products-v1'),UNIQUE(code,unit)
);
INSERT INTO org_fuel_products_v1 VALUES
 ('ETHANOL','L','ML','LIQUID_FUEL','ETHANOL','fuel-products-v1'),
 ('GASOLINE_REGULAR','L','ML','LIQUID_FUEL','GASOLINE','fuel-products-v1'),
 ('GASOLINE_ADDITIVED','L','ML','LIQUID_FUEL','GASOLINE','fuel-products-v1'),
 ('GASOLINE_PREMIUM_GRADE','L','ML','LIQUID_FUEL','GASOLINE','fuel-products-v1'),
 ('DIESEL_S10','L','ML','LIQUID_FUEL','DIESEL','fuel-products-v1'),
 ('DIESEL_S500','L','ML','LIQUID_FUEL','DIESEL','fuel-products-v1'),
 ('CNG','M3','MILLI_M3','GAS_FUEL','NATURAL_GAS','fuel-products-v1'),
 ('ELECTRICITY','KWH','WH','ELECTRICITY','ELECTRIC','fuel-products-v1'),
 ('ARLA_32','L','ML','ADDITIVE','NONE','fuel-products-v1');
CREATE FUNCTION org_asset_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='asset history is immutable';END $$;
CREATE TRIGGER org_fuel_products_v1_immutable BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON org_fuel_products_v1 FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
REVOKE ALL ON org_fuel_products_v1 FROM PUBLIC;

CREATE FUNCTION org_asset_text_valid(raw TEXT,maximum INTEGER,required BOOLEAN) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT raw IS NOT NULL AND (maximum=0 OR char_length(raw)<=maximum)
 AND raw !~ U&'[\0001-\001f\007f-\009f]'
 AND (NOT required OR length(translate(raw,U&' \00a0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200a\2028\2029\202f\205f\3000',''))>0);
$$;
CREATE FUNCTION org_asset_identifier(raw TEXT,plate BOOLEAN) RETURNS TEXT
 LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
DECLARE result TEXT;
BEGIN
 IF raw IS NULL OR raw='' THEN RETURN '';END IF;
 IF raw~'[^ -~]' OR (NOT plate AND octet_length(raw)>20) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset identifier';END IF;
 result=translate(translate(raw,' -',''),'abcdefghijklmnopqrstuvwxyz','ABCDEFGHIJKLMNOPQRSTUVWXYZ');
 IF result='' OR (plate AND (length(result)<>7 OR result~'[^A-Z0-9]')) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset identifier';END IF;
 RETURN result;
END $$;
CREATE TABLE org_asset_categories(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 label TEXT NOT NULL CHECK(org_asset_text_valid(label,0,true)),PRIMARY KEY(org_id,id)
);
-- An uncapped Unicode label cannot use a bounded btree key. Equality is checked
-- verbatim under the org root lock; this nonunique hash narrows lookup only.
CREATE INDEX org_asset_categories_label_lookup ON org_asset_categories(org_id,md5(label));
CREATE FUNCTION org_asset_category_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM public.org_asset_categories WHERE org_id=NEW.org_id AND label COLLATE "C"=NEW.label COLLATE "C") THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_asset_categories_exact_label',MESSAGE='asset category already declared';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_category_insert BEFORE INSERT ON org_asset_categories FOR EACH ROW EXECUTE FUNCTION org_asset_category_guard();
CREATE TRIGGER org_asset_category_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_categories FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE TABLE org_assets(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('VEHICLE','MACHINE','IMPLEMENT')),category_id UUID NOT NULL,
 name TEXT NOT NULL CHECK(org_asset_text_valid(name,80,true)),fleet_number TEXT NOT NULL DEFAULT '',plate TEXT NOT NULL DEFAULT '',serial_number TEXT NOT NULL DEFAULT '' CHECK(org_asset_text_valid(serial_number,40,false)),
 fleet_key TEXT NOT NULL DEFAULT '',plate_key TEXT NOT NULL DEFAULT '',usage_basis TEXT NOT NULL CHECK(usage_basis IN('DISTANCE','HOURS')),
 state TEXT NOT NULL CHECK(state IN('DRAFT','ACTIVE','INACTIVE','DISPOSED','ARCHIVED')),revision BIGINT NOT NULL CHECK(revision>0),
 unit_id UUID,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),CONSTRAINT org_asset_category_fkey FOREIGN KEY(org_id,category_id) REFERENCES org_asset_categories(org_id,id),
 CONSTRAINT org_asset_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CHECK(fleet_key<>'' OR plate_key<>'' OR org_asset_text_valid(serial_number,40,true))
);
CREATE UNIQUE INDEX org_assets_live_fleet_key ON org_assets(org_id,fleet_key COLLATE "C") WHERE state<>'ARCHIVED' AND fleet_key<>'';
CREATE UNIQUE INDEX org_assets_live_plate_key ON org_assets(org_id,plate_key COLLATE "C") WHERE state<>'ARCHIVED' AND plate_key<>'';
CREATE TABLE org_asset_tanks(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,position INTEGER NOT NULL CHECK(position>=0),
 role TEXT NOT NULL CHECK(role IN('MAIN','AUXILIARY','ADDITIVE','BATTERY')),capacity BIGINT NOT NULL CHECK(capacity BETWEEN 1 AND 1000000000),unit TEXT NOT NULL CHECK(unit IN('L','M3','KWH')),
 PRIMARY KEY(org_id,asset_id,position),UNIQUE(org_id,asset_id,position,unit),
 CONSTRAINT org_asset_tanks_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id) ON DELETE CASCADE
);
CREATE TABLE org_asset_tank_products(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,tank_position INTEGER NOT NULL,product TEXT NOT NULL,unit TEXT NOT NULL,
 PRIMARY KEY(org_id,asset_id,tank_position,product),
 CONSTRAINT org_asset_tank_product_tank_fkey FOREIGN KEY(org_id,asset_id,tank_position,unit) REFERENCES org_asset_tanks(org_id,asset_id,position,unit) ON DELETE CASCADE,
 FOREIGN KEY(product,unit) REFERENCES org_fuel_products_v1(code,unit)
);
CREATE TABLE org_asset_expected(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,position INTEGER NOT NULL CHECK(position>=0),product TEXT NOT NULL,basis TEXT NOT NULL CHECK(basis IN('DISTANCE','HOURS')),
 value BIGINT NOT NULL CHECK(value>0),unit TEXT NOT NULL,source TEXT NOT NULL CHECK(org_asset_text_valid(source,280,true)),reference_date DATE NOT NULL CHECK(reference_date>='0001-01-01'::date AND reference_date<'10000-01-01'::date),
 PRIMARY KEY(org_id,asset_id,product),UNIQUE(org_id,asset_id,position),CONSTRAINT org_asset_expected_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id) ON DELETE CASCADE,
 FOREIGN KEY(product,unit) REFERENCES org_fuel_products_v1(code,unit)
);
CREATE TABLE org_asset_unit_allocations(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,unit_id UUID NOT NULL,started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),ended_at TIMESTAMPTZ,
 PRIMARY KEY(org_id,asset_id,started_at),CONSTRAINT org_asset_allocation_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_allocation_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CHECK(isfinite(started_at) AND(ended_at IS NULL OR(isfinite(ended_at) AND ended_at>=started_at)))
);
CREATE UNIQUE INDEX org_asset_current_unit ON org_asset_unit_allocations(org_id,asset_id) WHERE ended_at IS NULL;
CREATE TABLE org_asset_revisions(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,revision BIGINT NOT NULL CHECK(revision>0),operation_id UUID NOT NULL,
 snapshot JSONB NOT NULL CHECK(jsonb_typeof(snapshot)='object'),created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(org_id,asset_id,revision),
 CONSTRAINT org_asset_revision_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id) ON DELETE CASCADE,
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE org_asset_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),asset_id UUID NOT NULL CHECK(asset_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('CREATE','CHANGE','DISCARD','ACTIVATE')),request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_state TEXT NOT NULL DEFAULT '',result_revision BIGINT NOT NULL CHECK(result_revision>0),discarded BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((kind='DISCARD' AND discarded AND result_state='' AND result_revision>1) OR(kind<>'DISCARD' AND NOT discarded AND result_state IN('DRAFT','ACTIVE') AND(kind<>'CREATE' OR(result_state='DRAFT' AND result_revision=1))))
);
CREATE TRIGGER org_asset_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_commands FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE TRIGGER org_asset_allocation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_unit_allocations FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE FUNCTION org_asset_revision_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='DELETE' AND pg_trigger_depth()>1 AND NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=OLD.org_id AND id=OLD.asset_id) AND OLD.snapshot->>'state'='DRAFT' THEN RETURN OLD;END IF;
 RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='asset revision history is immutable';
END $$;
CREATE TRIGGER org_asset_revisions_immutable BEFORE UPDATE OR DELETE ON org_asset_revisions FOR EACH ROW EXECUTE FUNCTION org_asset_revision_guard();
CREATE TRIGGER org_asset_revisions_no_truncate BEFORE TRUNCATE ON org_asset_revisions FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',0,'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset'));
END $$;
CREATE FUNCTION org_asset_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;root_state TEXT;usage JSONB;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept asset changes';END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.state<>'DRAFT' THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
  IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  RETURN OLD;
 END IF;
 IF NEW.fleet_key IS DISTINCT FROM public.org_asset_identifier(NEW.fleet_number,false) OR NEW.plate_key IS DISTINCT FROM public.org_asset_identifier(NEW.plate,true) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset identifier projection mismatch';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'DRAFT' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assets start as draft';END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset identity or revision';END IF;
  IF NEW.state IS DISTINCT FROM OLD.state AND NOT(OLD.state='DRAFT' AND NEW.state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset transition unavailable';END IF;
  IF NEW.kind<>OLD.kind THEN
   usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
   IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset kind fixed by accepted operation';END IF;
  END IF;
  IF NEW.state='ACTIVE' AND NEW.kind<>'IMPLEMENT' THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset meter source unavailable';END IF;
  IF OLD.state='ACTIVE' AND NEW.unit_id IS DISTINCT FROM OLD.unit_id THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='active asset unit change unavailable';END IF;
  IF NEW.state='ACTIVE' AND OLD.state='DRAFT' THEN
   IF NEW.kind<>'IMPLEMENT' THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset meter source unavailable';END IF;
   PERFORM public.org_unit_admit(NEW.org_id,NEW.unit_id);
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF NEW.unit_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_write BEFORE INSERT OR UPDATE OR DELETE ON org_assets FOR EACH ROW EXECUTE FUNCTION org_asset_guard();
CREATE TRIGGER org_assets_no_truncate BEFORE TRUNCATE ON org_assets FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE FUNCTION org_asset_configuration_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_asset UUID;root_state TEXT;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 selected_asset=CASE WHEN TG_OP='DELETE' THEN OLD.asset_id ELSE NEW.asset_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept asset changes';END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;
 IF TG_OP='UPDATE' AND(NEW.org_id<>OLD.org_id OR NEW.asset_id<>OLD.asset_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset configuration identity is immutable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_tank_guard BEFORE INSERT OR UPDATE OR DELETE ON org_asset_tanks FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_guard();
CREATE TRIGGER org_asset_product_guard BEFORE INSERT OR UPDATE OR DELETE ON org_asset_tank_products FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_guard();
CREATE TRIGGER org_asset_expected_guard BEFORE INSERT OR UPDATE OR DELETE ON org_asset_expected FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_guard();
CREATE FUNCTION org_asset_configuration_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_asset UUID;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 selected_asset=CASE WHEN TG_OP='DELETE' THEN OLD.asset_id ELSE NEW.asset_id END;
 IF EXISTS(SELECT 1 FROM public.org_asset_tanks t WHERE t.org_id=selected_org AND t.asset_id=selected_asset AND NOT EXISTS(SELECT 1 FROM public.org_asset_tank_products p WHERE(p.org_id,p.asset_id,p.tank_position)=(t.org_id,t.asset_id,t.position)))
 OR EXISTS(SELECT 1 FROM public.org_asset_tank_products p JOIN public.org_asset_tank_products q USING(org_id,asset_id,product) WHERE p.org_id=selected_org AND p.asset_id=selected_asset AND p.unit<>q.unit)
 OR EXISTS(SELECT 1 FROM public.org_asset_expected e WHERE e.org_id=selected_org AND e.asset_id=selected_asset AND NOT EXISTS(SELECT 1 FROM public.org_asset_tank_products p WHERE(p.org_id,p.asset_id,p.product,p.unit)=(e.org_id,e.asset_id,e.product,e.unit)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incomplete asset configuration';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_asset_tank_complete AFTER INSERT OR UPDATE OR DELETE ON org_asset_tanks DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_complete();
CREATE CONSTRAINT TRIGGER org_asset_product_complete AFTER INSERT OR UPDATE OR DELETE ON org_asset_tank_products DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_complete();
CREATE CONSTRAINT TRIGGER org_asset_expected_complete AFTER INSERT OR UPDATE OR DELETE ON org_asset_expected DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_configuration_complete();
CREATE FUNCTION org_asset_allocation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 PERFORM public.org_unit_admit(NEW.org_id,NEW.unit_id);
 IF NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id AND state='ACTIVE' AND unit_id=NEW.unit_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset allocation unavailable';END IF;
 NEW.started_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_asset_allocation_insert BEFORE INSERT ON org_asset_unit_allocations FOR EACH ROW EXECUTE FUNCTION org_asset_allocation_guard();
-- Preserve EMP-ORG-008's current employee UNION and DISTINCT semantics exactly.
CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;asset_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
 AND NOT((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
 OR(c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
 OR(c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
 OR(c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey')
 OR(c.conrelid='public.org_assets'::regclass AND c.conname='org_asset_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(*) INTO asset_count FROM public.org_asset_unit_allocations p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND p.ended_at IS NULL AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;
DO $$ DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_asset_categories','org_assets','org_asset_tanks','org_asset_tank_products','org_asset_expected','org_asset_unit_allocations','org_asset_revisions','org_asset_commands'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- Admit ACTIVE only together with its real matching first unit period.
CREATE FUNCTION org_asset_allocation_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_asset UUID;
BEGIN
 selected_org=NEW.org_id;IF TG_TABLE_NAME='org_assets' THEN selected_asset=NEW.id;ELSE selected_asset=NEW.asset_id;END IF;
 IF EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=selected_org AND a.id=selected_asset AND
 ((a.state='ACTIVE' AND(a.unit_id IS NULL OR(SELECT count(*) FROM public.org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id AND p.ended_at IS NULL AND p.unit_id=a.unit_id)<>1))
 OR(a.state='DRAFT' AND EXISTS(SELECT 1 FROM public.org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id AND p.ended_at IS NULL))))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset current allocation unavailable';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_asset_initial_allocation AFTER INSERT OR UPDATE ON org_assets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_allocation_complete();
CREATE CONSTRAINT TRIGGER org_asset_period_complete AFTER INSERT ON org_asset_unit_allocations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_allocation_complete();
