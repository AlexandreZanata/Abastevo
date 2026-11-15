-- EMP-TRIP-009. Manager-planned trips: destination, reason, customer or
-- project, cargo or passengers (B-BR-EMP-381..386, EDR-246..249). PLAN moves a
-- DRAFT, PLANNED, ASSIGNED or DECLINED mission to PLANNED, withdrawing its
-- assignee and answer, and appends one insert-only planning revision bound to
-- its PLAN transition and command receipt. A plan never touches a trip,
-- reading, event, review entry, reservation, block or asset.

-- The 227 kind/state CHECKs, widened by name; existing rows stay valid.
ALTER TABLE org_trip_mission_transitions DROP CONSTRAINT org_trip_mission_transitions_kind_check,
 ADD CONSTRAINT org_trip_mission_transitions_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','ACCEPT','DECLINE','CANCEL'));
ALTER TABLE org_trip_mission_commands DROP CONSTRAINT org_trip_mission_commands_kind_check,
 ADD CONSTRAINT org_trip_mission_commands_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','CANCEL')),
 DROP CONSTRAINT org_trip_mission_commands_result_state_check,
 ADD CONSTRAINT org_trip_mission_commands_result_state_check CHECK(result_state IN('DRAFT','PLANNED','ASSIGNED','CANCELLED'));

-- B-BR-EMP-382: the METER-001 text profile and no standalone CPF-shaped token
-- (the kernel.HasCPFShapedToken portal privacy shape).
CREATE FUNCTION org_trip_mission_plan_text_valid(raw TEXT) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT public.org_asset_text_valid(raw,280,true) AND raw !~ '(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)'
 $$;

-- One planning revision per PLAN transition (B-BR-EMP-383): the content, the
-- authorized planner and the company command that recorded it.
CREATE TABLE org_trip_mission_plans(
 org_id UUID NOT NULL,
 mission_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>=2),
 operation_id UUID NOT NULL,
 destination_text TEXT CHECK(destination_text IS NULL OR org_trip_mission_plan_text_valid(destination_text)),
 destination_unit_id UUID,
 reason TEXT NOT NULL CHECK(org_trip_mission_plan_text_valid(reason)),
 cost_centre_id UUID,
 load_kind TEXT NOT NULL CHECK(load_kind IN('NONE','CARGO','PASSENGERS')),
 cargo_description TEXT CHECK(cargo_description IS NULL OR org_trip_mission_plan_text_valid(cargo_description)),
 passenger_count INTEGER CHECK(passenger_count IS NULL OR passenger_count BETWEEN 1 AND 999),
 planned_by UUID NOT NULL REFERENCES accounts(id),
 planned_by_membership_id UUID NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,mission_id,revision),
 CONSTRAINT org_trip_mission_plans_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_mission_plans_transition_fkey FOREIGN KEY(org_id,mission_id,revision,operation_id)
  REFERENCES org_trip_mission_transitions(org_id,mission_id,revision,operation_id),
 CONSTRAINT org_trip_mission_plans_destination_unit_fkey FOREIGN KEY(org_id,destination_unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_trip_mission_plans_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_trip_mission_plans_planner_fkey FOREIGN KEY(org_id,planned_by_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_mission_plans_shape CHECK((destination_text IS NULL)<>(destination_unit_id IS NULL)
  AND CASE load_kind
   WHEN 'NONE' THEN cargo_description IS NULL AND passenger_count IS NULL
   WHEN 'CARGO' THEN cargo_description IS NOT NULL AND passenger_count IS NULL
   ELSE cargo_description IS NULL AND passenger_count IS NOT NULL END),
 CHECK(isfinite(recorded_at))
);

-- The planning revision of the PLAN transition of the same revision and
-- operation, by its current planner, naming an ACTIVE unit and cost centre.
CREATE FUNCTION org_trip_mission_plan_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trip_mission_transitions WHERE org_id=NEW.org_id AND mission_id=NEW.mission_id AND revision=NEW.revision;
 IF NOT FOUND OR t.kind<>'PLAN' OR t.operation_id<>NEW.operation_id OR t.actor_account_id<>NEW.planned_by OR t.actor_membership_id<>NEW.planned_by_membership_id
 OR NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.planned_by_membership_id,NEW.planned_by)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission plan source unavailable';END IF;
 IF NEW.destination_unit_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.destination_unit_id AND u.state='ACTIVE')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission plan unit unavailable';END IF;
 IF NEW.cost_centre_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_cost_centres c WHERE c.org_id=NEW.org_id AND c.id=NEW.cost_centre_id AND c.state='ACTIVE')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission plan cost centre unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_plan_source BEFORE INSERT ON org_trip_mission_plans FOR EACH ROW EXECUTE FUNCTION org_trip_mission_plan_source();

ALTER TABLE org_trip_mission_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_trip_mission_plans FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_mission_plans_scope ON org_trip_mission_plans USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_mission_plans_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_mission_plans FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_mission_plans FROM PUBLIC;

-- The 227 table with the EDR-246 PLAN rows.
CREATE OR REPLACE FUNCTION org_trip_mission_next(from_state TEXT,command TEXT) RETURNS TEXT
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT CASE
  WHEN from_state IN('COMPLETED','CANCELLED','IN_PROGRESS') THEN NULL
  WHEN command='PLAN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','DECLINED') THEN 'PLANNED'
  WHEN command='ASSIGN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'ASSIGNED'
  WHEN command='CANCEL' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'CANCELLED'
  WHEN command='ACCEPT' AND from_state='ASSIGNED' THEN 'ACCEPTED'
  WHEN command='DECLINE' AND from_state='ASSIGNED' THEN 'DECLINED'
 END
 $$;

-- The 227 transition source; a PLAN carries no assignee (it withdraws the
-- current one) and every other kind keeps the current assignee as before.
CREATE OR REPLACE FUNCTION org_trip_mission_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_trip_missions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.kind='CREATE' THEN
  IF NEW.revision<>1 OR m.revision<>1 OR m.state<>'DRAFT' OR NEW.operation_id<>m.create_operation_id OR NEW.actor_account_id<>m.created_by
  OR NEW.actor_membership_id<>m.created_by_membership_id OR NEW.source<>'COMPANY'
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
 ELSE
  IF NEW.revision<>m.revision+1 OR NEW.from_state IS DISTINCT FROM m.state OR NEW.to_state IS DISTINCT FROM public.org_trip_mission_next(m.state,NEW.kind)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind='ASSIGN' AND(NOT public.org_trip_mission_assignee_valid(NEW.org_id,NEW.assignee_employee_id,NEW.assignee_membership_id)
   OR m.state IN('ASSIGNED','ACCEPTED') AND NEW.assignee_membership_id=m.assignee_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission assignee unavailable';END IF;
  IF NEW.kind='PLAN' AND(NEW.assignee_employee_id IS NOT NULL OR NEW.assignee_membership_id IS NOT NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind NOT IN('ASSIGN','PLAN') AND(NEW.assignee_employee_id IS DISTINCT FROM m.assignee_employee_id OR NEW.assignee_membership_id IS DISTINCT FROM m.assignee_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind IN('ACCEPT','DECLINE') AND(NEW.actor_membership_id IS DISTINCT FROM m.assignee_membership_id OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships mm ON(mm.org_id,mm.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments d ON(d.org_id,d.id,d.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.mission.respond' AND o.operation_schema=1
   AND o.entity_type='mission' AND o.entity_id=NEW.mission_id AND o.membership_id=NEW.actor_membership_id AND o.enrollment_id=NEW.enrollment_id
   AND o.owner_account_id=NEW.actor_account_id AND mm.account_id=o.owner_account_id AND mm.state='ACTIVE' AND d.state='ENROLLED'))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission response source unavailable';END IF;
 END IF;
 IF NEW.source='COMPANY' AND NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.actor_membership_id,NEW.actor_account_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;

-- The 227 head guard; a PLAN moves the head to PLANNED without assignee or
-- answer and only after its planning revision exists.
CREATE OR REPLACE FUNCTION org_trip_mission_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.id IS DISTINCT FROM OLD.id OR NEW.unit_id IS DISTINCT FROM OLD.unit_id OR NEW.asset_id IS DISTINCT FROM OLD.asset_id
 OR NEW.title IS DISTINCT FROM OLD.title OR NEW.starts_at IS DISTINCT FROM OLD.starts_at OR NEW.ends_at IS DISTINCT FROM OLD.ends_at
 OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_by_membership_id IS DISTINCT FROM OLD.created_by_membership_id
 OR NEW.create_operation_id IS DISTINCT FROM OLD.create_operation_id OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.revision<>OLD.revision+1
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 SELECT * INTO t FROM public.org_trip_mission_transitions WHERE org_id=NEW.org_id AND mission_id=NEW.id AND revision=NEW.revision;
 IF NOT FOUND OR t.from_state IS DISTINCT FROM OLD.state OR t.to_state IS DISTINCT FROM NEW.state
 OR t.assignee_employee_id IS DISTINCT FROM NEW.assignee_employee_id OR t.assignee_membership_id IS DISTINCT FROM NEW.assignee_membership_id
 OR t.kind='ASSIGN' AND(NEW.answer IS NOT NULL OR NEW.answer_reason IS NOT NULL)
 OR t.kind IN('ACCEPT','DECLINE') AND(NEW.answer IS DISTINCT FROM t.kind OR NEW.answer_reason IS DISTINCT FROM t.reason)
 OR t.kind='CANCEL' AND(NEW.answer IS DISTINCT FROM OLD.answer OR NEW.answer_reason IS DISTINCT FROM OLD.answer_reason)
 OR t.kind='PLAN' AND(NEW.answer IS NOT NULL OR NEW.answer_reason IS NOT NULL OR NOT EXISTS(SELECT 1 FROM public.org_trip_mission_plans p
  WHERE p.org_id=t.org_id AND p.mission_id=t.mission_id AND p.revision=t.revision AND p.operation_id=t.operation_id))
 OR t.kind='CREATE'
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;

-- The 227 publication check; a PLAN transition is published only with its
-- planning revision.
CREATE OR REPLACE FUNCTION org_trip_mission_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_trip_missions' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_transitions t WHERE t.org_id=NEW.org_id AND t.mission_id=NEW.id AND t.revision=1 AND t.kind='CREATE')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM public.org_trip_missions m WHERE m.org_id=NEW.org_id AND m.id=NEW.mission_id AND m.revision>=NEW.revision)
 OR NEW.source='COMPANY' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_commands c WHERE c.org_id=NEW.org_id AND c.operation_id=NEW.operation_id
  AND c.mission_id=NEW.mission_id AND c.result_revision=NEW.revision AND c.actor_id=NEW.actor_account_id)
 OR NEW.source='DEVICE' AND NOT public.org_trip_published(NEW.org_id,NEW.operation_id,NEW.revision)
 OR NEW.kind='PLAN' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_plans p WHERE p.org_id=NEW.org_id AND p.mission_id=NEW.mission_id
  AND p.revision=NEW.revision AND p.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;

-- The 230 unit-use source (every line kept) with the plan destination unit
-- recognised as an association, never as current use (EDR-248).
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
 OR(c.conrelid='public.org_trip_mission_plans'::regclass AND c.conname='org_trip_mission_plans_destination_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
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

-- The 230 cost-centre-use source (every line kept) with the plan cost centre
-- recognised as an association, never as current use (EDR-248).
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
   OR(c.conrelid='public.org_trip_mission_plans'::regclass AND c.conname='org_trip_mission_plans_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
