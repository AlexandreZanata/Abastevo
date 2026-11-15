-- EMP-FUEL-002. One immutable organization fueling per signed fueling.submit
-- operation (B-BR-EMP-263..267, EDR-175..177). The original revision-1 fact,
-- its arithmetic reconciliation, its warnings and its first APPR-001 review
-- entry are insert-only. A reading is fleet evidence (origin FUELING) stored
-- in the same transaction; review decisions, amendments, expenses, document
-- links and price snapshots belong to later owners.
CREATE TABLE org_fuelings(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 asset_id UUID NOT NULL,
 responsible_employee_id UUID NOT NULL,
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 product TEXT NOT NULL,
 unit TEXT NOT NULL,
 quantity BIGINT NOT NULL CHECK(quantity BETWEEN 1 AND 1000000000),
 paid_centavos BIGINT NOT NULL CHECK(paid_centavos BETWEEN 0 AND 1000000000000),
 discount_centavos BIGINT CHECK(discount_centavos BETWEEN 0 AND 1000000000000),
 unit_price_milli_brl BIGINT CHECK(unit_price_milli_brl BETWEEN 1 AND 1000000),
 unit_price_text TEXT CHECK(octet_length(unit_price_text) BETWEEN 1 AND 8192),
 fill TEXT NOT NULL CHECK(fill IN('FULL','PARTIAL','UNKNOWN')),
 payer TEXT NOT NULL CHECK(payer IN('COMPANY','EMPLOYEE')),
 payment_method TEXT NOT NULL CHECK(payment_method IN('CASH','PIX','DEBIT_CARD','CREDIT_CARD','FUEL_CARD','SUPPLIER_ACCOUNT','AGREEMENT_VOUCHER','INTERNAL_STOCK','OTHER')),
 origin TEXT NOT NULL CHECK(origin='APP'),
 place_kind TEXT CHECK(place_kind='NOT_FOUND'),
 place_name TEXT,
 meter_id UUID,
 segment_id UUID,
 reading_id UUID,
 missing_reading_reason TEXT,
 unit_id UUID,
 cost_centre_id UUID,
 gross_centavos BIGINT NOT NULL,
 expected_centavos BIGINT,
 difference_centavos BIGINT,
 derived_price_milli_brl TEXT CHECK(derived_price_milli_brl~'^(0|[1-9][0-9]*)(/[1-9][0-9]*)?$'),
 review_state TEXT NOT NULL CHECK(review_state='SUBMITTED'),
 revision BIGINT NOT NULL CHECK(revision=1),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_fuelings_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_fuelings_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_fuelings_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_fuelings_responsible_fkey FOREIGN KEY(org_id,responsible_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_fuelings_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_fuelings_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_fuelings_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_fuelings_product_fkey FOREIGN KEY(product,unit) REFERENCES org_fuel_products_v1(code,unit),
 CONSTRAINT org_fuelings_reading_fkey FOREIGN KEY(org_id,meter_id,reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_fuelings_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_fuelings_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_fuelings_reading_evidence CHECK(num_nonnulls(reading_id,missing_reading_reason)=1
  AND num_nonnulls(meter_id,segment_id,reading_id) IN(0,3)
  AND(missing_reading_reason IS NULL OR org_asset_text_valid(missing_reading_reason,280,true))),
 CONSTRAINT org_fuelings_place CHECK((place_kind IS NULL AND place_name IS NULL) OR(place_kind='NOT_FOUND' AND org_asset_text_valid(place_name,0,true))),
 CONSTRAINT org_fuelings_snapshot CHECK(cost_centre_id IS NULL OR unit_id IS NOT NULL),
 -- BR-MONEY-004/005: the paid total is the entered fact; the reconciliation
 -- is re-proved here and an absent discount reconciles as zero.
 CONSTRAINT org_fuelings_arithmetic CHECK(gross_centavos=paid_centavos+COALESCE(discount_centavos,0)
  AND(unit_price_milli_brl IS NULL)=(expected_centavos IS NULL) AND(unit_price_milli_brl IS NULL)=(difference_centavos IS NULL)
  AND(unit_price_milli_brl IS NULL)=(derived_price_milli_brl IS NOT NULL)
  AND(unit_price_milli_brl IS NULL OR(expected_centavos=(quantity*unit_price_milli_brl+5000)/10000 AND difference_centavos=abs(gross_centavos-expected_centavos)))
  AND(unit_price_text IS NULL OR unit_price_milli_brl IS NOT NULL)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE INDEX org_fuelings_asset ON org_fuelings(org_id,asset_id,occurred_at,occurred_submicro,id);

CREATE TABLE org_fueling_warnings(
 org_id UUID NOT NULL,fueling_id UUID NOT NULL,revision BIGINT NOT NULL CHECK(revision=1),
 code TEXT NOT NULL CHECK(code IN('ARITHMETIC_MISMATCH','PRODUCT_NOT_ACCEPTED')),
 PRIMARY KEY(org_id,fueling_id,revision,code),
 CONSTRAINT org_fueling_warnings_fueling_fkey FOREIGN KEY(org_id,fueling_id) REFERENCES org_fuelings(org_id,id)
);
-- APPR-001 history of one fueling; FUEL-003 appends its decisions.
CREATE TABLE org_fueling_review_entries(
 org_id UUID NOT NULL,fueling_id UUID NOT NULL,sequence BIGINT NOT NULL,
 kind TEXT NOT NULL,revision BIGINT NOT NULL,state TEXT NOT NULL,reason TEXT NOT NULL,
 actor_membership_id UUID NOT NULL,operation_id UUID NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,fueling_id,sequence),
 CONSTRAINT org_fueling_review_entries_fueling_fkey FOREIGN KEY(org_id,fueling_id) REFERENCES org_fuelings(org_id,id),
 CONSTRAINT org_fueling_review_entries_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_fueling_review_entries_submit CHECK(sequence=1 AND kind='SUBMIT' AND revision=1 AND state='SUBMITTED' AND reason=''),
 CHECK(isfinite(recorded_at))
);

CREATE FUNCTION org_fueling_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;asset_state TEXT;actual_unit UUID;actual_centre UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR o.operation_type<>'fueling.submit' OR o.operation_schema<>1 OR o.entity_type<>'fueling' OR o.entity_id<>NEW.id
 OR o.payload_digest<>NEW.payload_digest OR o.membership_id<>NEW.author_membership_id OR o.enrollment_id<>NEW.enrollment_id
 OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_device_enrollments e ON(e.org_id,e.membership_id)=(m.org_id,m.id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.author_membership_id AND m.employee_id=NEW.author_employee_id AND m.account_id=o.owner_account_id AND e.id=NEW.enrollment_id AND e.state='ENROLLED')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling source unavailable';END IF;
 SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF asset_state='DRAFT' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='draft asset does not accept fuelings';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.responsible_employee_id AND state='ACTIVE' AND merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='responsible employee unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid fueling declaration';END IF;
 -- BR-AUTH-005: the stored snapshot is the FLEET-005 allocation at the
 -- declared instant (microsecond floor, half-open periods), never current.
 SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling allocation snapshot unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_fueling_source BEFORE INSERT ON org_fuelings FOR EACH ROW EXECUTE FUNCTION org_fueling_source();

CREATE FUNCTION org_fueling_child_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_TABLE_NAME='org_fueling_review_entries' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_fuelings f WHERE f.org_id=NEW.org_id AND f.id=NEW.fueling_id AND f.operation_id=NEW.operation_id AND f.author_membership_id=NEW.actor_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling review source unavailable';END IF;
  NEW.recorded_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_fueling_warning_source BEFORE INSERT ON org_fueling_warnings FOR EACH ROW EXECUTE FUNCTION org_fueling_child_source();
CREATE TRIGGER org_fueling_review_source BEFORE INSERT ON org_fueling_review_entries FOR EACH ROW EXECUTE FUNCTION org_fueling_child_source();

-- At commit a fueling is published only with its accepted receipt, its
-- first review entry, its exact arithmetic warning and its reading evidence.
CREATE FUNCTION org_fueling_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE f public.org_fuelings%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT * INTO f FROM public.org_fuelings WHERE org_id=NEW.org_id AND id=NEW.id;
 IF NOT FOUND
 OR NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a JOIN public.org_sync_effects e USING(org_id,operation_id)
  WHERE a.org_id=f.org_id AND a.operation_id=f.operation_id AND a.result='ACCEPTED' AND a.entity_revision=1 AND e.entity_revision=1 AND a.received_at>=f.received_at)
 OR NOT EXISTS(SELECT 1 FROM public.org_fueling_review_entries r WHERE r.org_id=f.org_id AND r.fueling_id=f.id AND r.sequence=1)
 OR(COALESCE(f.difference_centavos,0)>1)<>EXISTS(SELECT 1 FROM public.org_fueling_warnings w WHERE w.org_id=f.org_id AND w.fueling_id=f.id AND w.code='ARITHMETIC_MISMATCH')
 OR(f.reading_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id)
  WHERE r.org_id=f.org_id AND r.meter_id=f.meter_id AND r.id=f.reading_id AND r.segment_id=f.segment_id AND m.asset_id=f.asset_id
  AND r.origin='FUELING' AND r.operation_id=f.operation_id AND r.author_membership_id=f.author_membership_id AND r.capture_kind='ENROLLED_DEVICE' AND r.enrollment_id=f.enrollment_id
  AND(r.occurred_at,r.occurred_submicro)=(f.occurred_at,f.occurred_submicro) AND r.entry_zone=f.entry_zone AND r.device_offset_seconds=f.device_offset_seconds))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_fueling_complete AFTER INSERT ON org_fuelings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_fueling_complete();

CREATE FUNCTION org_fueling_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization fueling history is insert-only';END $$;
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_fuelings','org_fueling_warnings','org_fueling_review_entries'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_fueling_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- The 210 asset-use source with the fueling recognised as an accepted
-- operation on the asset: a fueling fixes the asset kind and keeps a draft
-- from being discarded (it never references a DRAFT asset at insertion).
CREATE OR REPLACE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey')
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_asset_fkey')
 OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_asset_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_asset_fkey')
 OR(c.conrelid=to_regclass('public.org_document_links') AND c.conname='org_document_links_asset_fkey')
 OR(c.conrelid='public.org_asset_assignments'::regclass AND c.conname='org_asset_assignments_asset_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset));
END $$;

-- The 208 unit-use source with the fueling's historical unit snapshot
-- recognised as an association, never as current use (BR-AUTH-005).
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
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_unit_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_unit_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(DISTINCT p.asset_id) INTO asset_count FROM public.org_asset_unit_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;

-- The 208 cost-centre-use source with the fueling's historical snapshot
-- recognised as an association, never as current use.
CREATE OR REPLACE FUNCTION org_cost_centre_current_usage(selected_org UUID,selected_centre UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass)
  AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey')
   OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey')
   OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey')
   OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_centre_fkey')
   OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_cost_centre_fkey')
   OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
