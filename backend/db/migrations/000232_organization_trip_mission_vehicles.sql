-- EMP-TRIP-013. Vehicle change within a mission (B-BR-EMP-387..389,
-- EDR-250, EDR-251). A change replaces the mission's vehicle at the next
-- revision, keeping its state (PLANNED, ASSIGNED, ACCEPTED or DECLINED),
-- assignee, answer and plan, and appends one
-- insert-only record of the previous and new vehicle bound to its VEHICLE
-- transition and command receipt. It never moves a trip, reading, meter,
-- event, review entry, reservation or block: a change is refused while the
-- assignee runs a trip on the previous vehicle, and the new vehicle must not
-- run another trip.

-- The 231 kind/state CHECKs, widened by name; existing rows stay valid.
ALTER TABLE org_trip_mission_transitions DROP CONSTRAINT org_trip_mission_transitions_kind_check,
 ADD CONSTRAINT org_trip_mission_transitions_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','ACCEPT','DECLINE','CANCEL','VEHICLE'));
ALTER TABLE org_trip_mission_commands DROP CONSTRAINT org_trip_mission_commands_kind_check,
 ADD CONSTRAINT org_trip_mission_commands_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','CANCEL','VEHICLE')),
 DROP CONSTRAINT org_trip_mission_commands_result_state_check,
 ADD CONSTRAINT org_trip_mission_commands_result_state_check CHECK(result_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','CANCELLED'));

-- One record per VEHICLE transition (B-BR-EMP-389): the vehicle before and
-- after, the reason, the authorized planner and the company command.
CREATE TABLE org_trip_mission_vehicles(
 org_id UUID NOT NULL,
 mission_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>=2),
 operation_id UUID NOT NULL,
 previous_asset_id UUID,
 asset_id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(org_asset_text_valid(reason,280,true)),
 changed_by UUID NOT NULL REFERENCES accounts(id),
 changed_by_membership_id UUID NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,mission_id,revision),
 CONSTRAINT org_trip_mission_vehicles_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_mission_vehicles_transition_fkey FOREIGN KEY(org_id,mission_id,revision,operation_id)
  REFERENCES org_trip_mission_transitions(org_id,mission_id,revision,operation_id),
 CONSTRAINT org_trip_mission_vehicles_previous_fkey FOREIGN KEY(org_id,previous_asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_trip_mission_vehicles_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_trip_mission_vehicles_planner_fkey FOREIGN KEY(org_id,changed_by_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_mission_vehicles_change CHECK(previous_asset_id IS DISTINCT FROM asset_id),
 CHECK(isfinite(recorded_at))
);

-- The record of the VEHICLE transition of the same revision and operation by
-- its current planner; the previous vehicle is the mission's current one;
-- the new one is an ACTIVE main asset running no trip; the assignee runs no
-- trip on the previous vehicle (B-BR-EMP-388).
CREATE FUNCTION org_trip_mission_vehicle_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;m public.org_trip_missions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trip_mission_transitions WHERE org_id=NEW.org_id AND mission_id=NEW.mission_id AND revision=NEW.revision;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 IF t.kind IS DISTINCT FROM 'VEHICLE' OR t.operation_id<>NEW.operation_id OR t.actor_account_id<>NEW.changed_by OR t.actor_membership_id<>NEW.changed_by_membership_id
 OR NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.changed_by_membership_id,NEW.changed_by)
 OR m.revision IS DISTINCT FROM NEW.revision-1 OR m.asset_id IS DISTINCT FROM NEW.previous_asset_id
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle source unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id AND a.state='ACTIVE' AND a.kind<>'IMPLEMENT')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trips r WHERE r.org_id=NEW.org_id AND r.asset_id=NEW.asset_id AND r.state='IN_PROGRESS')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle in use';END IF;
 IF NEW.previous_asset_id IS NOT NULL AND m.assignee_employee_id IS NOT NULL AND EXISTS(SELECT 1 FROM public.org_trips r WHERE r.org_id=NEW.org_id
  AND r.asset_id=NEW.previous_asset_id AND r.driver_employee_id=m.assignee_employee_id AND r.state='IN_PROGRESS')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission trip in progress';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_vehicle_source BEFORE INSERT ON org_trip_mission_vehicles FOR EACH ROW EXECUTE FUNCTION org_trip_mission_vehicle_source();

ALTER TABLE org_trip_mission_vehicles ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_trip_mission_vehicles FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_mission_vehicles_scope ON org_trip_mission_vehicles USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_mission_vehicles_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_mission_vehicles FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_mission_vehicles FROM PUBLIC;

-- The 231 table with the EDR-250 VEHICLE row: a PLANNED, ASSIGNED, ACCEPTED
-- or DECLINED mission keeps its state (a DRAFT exists only at revision 1).
CREATE OR REPLACE FUNCTION org_trip_mission_next(from_state TEXT,command TEXT) RETURNS TEXT
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT CASE
  WHEN from_state IN('COMPLETED','CANCELLED','IN_PROGRESS') THEN NULL
  WHEN command='PLAN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','DECLINED') THEN 'PLANNED'
  WHEN command='ASSIGN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'ASSIGNED'
  WHEN command='CANCEL' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'CANCELLED'
  WHEN command='ACCEPT' AND from_state='ASSIGNED' THEN 'ACCEPTED'
  WHEN command='DECLINE' AND from_state='ASSIGNED' THEN 'DECLINED'
  WHEN command='VEHICLE' AND from_state IN('PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN from_state
 END
 $$;

-- The 231 head guard; the vehicle changes only together with the record of
-- its VEHICLE revision, which keeps the answer.
CREATE OR REPLACE FUNCTION org_trip_mission_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.id IS DISTINCT FROM OLD.id OR NEW.unit_id IS DISTINCT FROM OLD.unit_id
 OR NEW.asset_id IS DISTINCT FROM OLD.asset_id AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v WHERE v.org_id=NEW.org_id AND v.mission_id=NEW.id
  AND v.revision=NEW.revision AND v.previous_asset_id IS NOT DISTINCT FROM OLD.asset_id AND v.asset_id=NEW.asset_id)
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
 OR t.kind='VEHICLE' AND(NEW.answer IS DISTINCT FROM OLD.answer OR NEW.answer_reason IS DISTINCT FROM OLD.answer_reason OR NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v
  WHERE v.org_id=t.org_id AND v.mission_id=t.mission_id AND v.revision=t.revision AND v.operation_id=t.operation_id AND v.asset_id=NEW.asset_id))
 OR t.kind='CREATE'
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;

-- The 231 publication check; a VEHICLE transition is published only with its
-- record.
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
 OR NEW.kind='VEHICLE' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v WHERE v.org_id=NEW.org_id AND v.mission_id=NEW.mission_id
  AND v.revision=NEW.revision AND v.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;

-- The 230 asset-use source (every line kept) with the two vehicle-change keys
-- recognised as associations, never as current use (EDR-251).
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
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;
