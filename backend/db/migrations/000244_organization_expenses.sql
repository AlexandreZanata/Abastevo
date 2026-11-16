-- EMP-EXP-002. One immutable organization expense per cost origin
-- (B-BR-EMP-444..449, EDR-287..290). A FUELING expense is projected from its
-- stored fueling inside the fueling.submit transaction; a DIRECT expense is
-- recorded by the signed expense.submit operation. The original revision-1
-- fact is insert-only; review decisions, amendments, allocations, payables
-- and payments belong to later owners and never insert a second cost.
CREATE TABLE org_expenses(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 origin_kind TEXT NOT NULL CHECK(origin_kind IN('FUELING','DIRECT')),
 source_id UUID NOT NULL,
 fueling_id UUID,
 source_revision BIGINT NOT NULL CHECK(source_revision=1),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 asset_id UUID NOT NULL,
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 category TEXT NOT NULL,
 paid_centavos BIGINT NOT NULL CHECK(paid_centavos BETWEEN 0 AND 1000000000000),
 company_centavos BIGINT NOT NULL CHECK(company_centavos BETWEEN 0 AND 1000000000000),
 payer TEXT NOT NULL CHECK(payer IN('COMPANY','EMPLOYEE')),
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 reporting_day DATE NOT NULL,
 reporting_zone TEXT NOT NULL,
 reporting_source TEXT NOT NULL CHECK(reporting_source IN('organization','unit')),
 unit_id UUID,
 cost_centre_id UUID,
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 review_state TEXT NOT NULL CHECK(review_state='SUBMITTED'),
 revision BIGINT NOT NULL CHECK(revision=1),
 PRIMARY KEY(org_id,id),
 -- EDR-131/BR-EXP-001: one cost per origin; 06-domain-model §5: one cost per fueling.
 CONSTRAINT org_expenses_origin_key UNIQUE(org_id,origin_kind,source_id),
 CONSTRAINT org_expenses_one_cost_per_fueling UNIQUE(org_id,fueling_id),
 CONSTRAINT org_expenses_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_expenses_origin_shape CHECK((origin_kind='FUELING' AND fueling_id IS NOT NULL AND source_id=fueling_id)
  OR(origin_kind='DIRECT' AND fueling_id IS NULL AND source_id=id)),
 CONSTRAINT org_expenses_company_within_paid CHECK(company_centavos<=paid_centavos),
 -- The v1 categories until EMP-EXP-003 replaces this constraint (EDR-288).
 CONSTRAINT org_expenses_category_v1 CHECK(category~'^[A-Z][A-Z0-9_]{0,63}$' AND((origin_kind='FUELING' AND category='FUEL')
  OR(origin_kind='DIRECT' AND category IN('TOLL','PARKING','MAINTENANCE','LEASE','INSURANCE','FINE','OTHER')))),
 CONSTRAINT org_expenses_snapshot CHECK(cost_centre_id IS NULL OR unit_id IS NOT NULL),
 CONSTRAINT org_expenses_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_expenses_fueling_fkey FOREIGN KEY(org_id,fueling_id) REFERENCES org_fuelings(org_id,id),
 CONSTRAINT org_expenses_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_expenses_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_expenses_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_expenses_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_expenses_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_expenses_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE INDEX org_expenses_reporting ON org_expenses(org_id,reporting_day,id);
CREATE INDEX org_expenses_asset ON org_expenses(org_id,asset_id,occurred_at,occurred_submicro,id);

-- The source proof: a FUELING expense mirrors its stored fueling exactly; a
-- DIRECT expense matches its signed expense.submit operation, a non-draft
-- asset and the FLEET-005 snapshot at its instant. Both re-prove the
-- reporting attribution (BR-TIME-004) and take the server receipt time.
CREATE FUNCTION org_expense_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE f public.org_fuelings%ROWTYPE;o public.org_sync_operations%ROWTYPE;asset_state TEXT;actual_unit UUID;actual_centre UUID;zone TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.origin_kind='FUELING' THEN
  SELECT * INTO f FROM public.org_fuelings WHERE org_id=NEW.org_id AND id=NEW.fueling_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  IF f.operation_id<>NEW.operation_id OR f.payload_digest<>NEW.payload_digest OR f.revision<>NEW.source_revision OR f.asset_id<>NEW.asset_id
  OR f.author_membership_id<>NEW.author_membership_id OR f.author_employee_id<>NEW.author_employee_id OR f.enrollment_id<>NEW.enrollment_id
  OR(f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds)<>(NEW.occurred_at,NEW.occurred_submicro,NEW.entry_zone,NEW.device_offset_seconds)
  OR NEW.category<>'FUEL' OR f.paid_centavos<>NEW.paid_centavos OR NEW.company_centavos<>f.paid_centavos OR f.payer<>NEW.payer
  OR f.unit_id IS DISTINCT FROM NEW.unit_id OR f.cost_centre_id IS DISTINCT FROM NEW.cost_centre_id OR f.review_state<>NEW.review_state
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling expense source unavailable';END IF;
 ELSE
  SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
  IF NOT FOUND OR o.operation_type<>'expense.submit' OR o.operation_schema<>1 OR o.entity_type<>'expense' OR o.entity_id<>NEW.id
  OR o.payload_digest<>NEW.payload_digest OR o.membership_id<>NEW.author_membership_id OR o.enrollment_id<>NEW.enrollment_id
  OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_device_enrollments e ON(e.org_id,e.membership_id)=(m.org_id,m.id)
   WHERE m.org_id=NEW.org_id AND m.id=NEW.author_membership_id AND m.employee_id=NEW.author_employee_id AND m.account_id=o.owner_account_id AND e.id=NEW.enrollment_id AND e.state='ENROLLED')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='expense source unavailable';END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  IF asset_state='DRAFT' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='draft asset does not accept expenses';END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid expense declaration';END IF;
  -- BR-AUTH-005: the FLEET-005 allocation at the competence instant.
  SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
  SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
  IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='expense allocation snapshot unavailable';END IF;
 END IF;
 IF NEW.unit_id IS NULL THEN SELECT time_zone INTO zone FROM public.organizations WHERE org_id=NEW.org_id;
 ELSE SELECT time_zone INTO zone FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id;END IF;
 IF zone IS NULL OR NEW.reporting_zone<>zone OR NEW.reporting_source<>(CASE WHEN NEW.unit_id IS NULL THEN 'organization' ELSE 'unit' END)
 OR NEW.reporting_day<>(NEW.occurred_at AT TIME ZONE zone)::date THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='expense reporting attribution unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_expense_source BEFORE INSERT ON org_expenses FOR EACH ROW EXECUTE FUNCTION org_expense_source();

-- At commit an expense is published only with the ACCEPTED receipt of the
-- operation that created it (for a FUELING expense: its fueling's).
CREATE FUNCTION org_expense_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE x public.org_expenses%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT * INTO x FROM public.org_expenses WHERE org_id=NEW.org_id AND id=NEW.id;
 IF NOT FOUND
 OR NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a JOIN public.org_sync_effects e USING(org_id,operation_id)
  WHERE a.org_id=x.org_id AND a.operation_id=x.operation_id AND a.result='ACCEPTED' AND a.entity_revision=1 AND e.entity_revision=1 AND(x.origin_kind='FUELING' OR a.received_at>=x.received_at))
 OR(x.origin_kind='FUELING' AND NOT EXISTS(SELECT 1 FROM public.org_fuelings f WHERE f.org_id=x.org_id AND f.id=x.fueling_id AND f.operation_id=x.operation_id))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='expense publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_expense_complete AFTER INSERT ON org_expenses DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_expense_complete();

-- EDR-287: a fueling accepted from now on never commits without its one
-- FUELING expense (BR-FUEL-010); the reverse is the composite foreign key.
CREATE FUNCTION org_fueling_expense_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_expenses x WHERE x.org_id=NEW.org_id AND x.fueling_id=NEW.id AND x.origin_kind='FUELING' AND x.source_revision=NEW.revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling cost unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_fueling_expense_complete AFTER INSERT ON org_fuelings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_fueling_expense_complete();

CREATE FUNCTION org_expense_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization expense history is insert-only';END $$;
ALTER TABLE org_expenses ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_expenses FORCE ROW LEVEL SECURITY;
CREATE POLICY org_expenses_scope ON org_expenses USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_expenses_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_expenses FOR EACH STATEMENT EXECUTE FUNCTION org_expense_keep_history();
REVOKE ALL ON org_expenses FROM PUBLIC;

-- The 232 asset-use source (every line kept) with the expense asset key
-- recognised as an association, never as current use: an expense names only
-- non-draft assets, so it never decides a draft discard or a kind change.
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
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_asset_fkey')
 OR(c.conrelid='public.org_asset_reservations'::regclass AND c.conname='org_asset_reservations_asset_fkey')
 OR(c.conrelid='public.org_asset_blocks'::regclass AND c.conname='org_asset_blocks_asset_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_asset_fkey')
 OR(c.conrelid='public.org_trip_held_starts'::regclass AND c.conname='org_trip_held_starts_asset_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_implement_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_powered_fkey')
 OR(c.conrelid='public.org_checklist_template_targets'::regclass AND c.conname='org_checklist_template_targets_asset_fkey')
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_asset_fkey')
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_asset_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_previous_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_asset_fkey')
 OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;

-- The 231 unit-use source (every line kept) with the expense's historical
-- unit snapshot recognised as an association, never as current use.
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
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_unit_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_unit_fkey')
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_unit_fkey')
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_unit_fkey')
 OR(c.conrelid='public.org_trip_mission_plans'::regclass AND c.conname='org_trip_mission_plans_destination_unit_fkey')
 OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
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

-- The 231 cost-centre-use source (every line kept) with the expense's
-- historical snapshot recognised as an association, never as current use.
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
   OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_cost_centre_fkey')
   OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_cost_centre_fkey')
   OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_cost_centre_fkey')
   OR(c.conrelid='public.org_trip_mission_plans'::regclass AND c.conname='org_trip_mission_plans_cost_centre_fkey')
   OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
