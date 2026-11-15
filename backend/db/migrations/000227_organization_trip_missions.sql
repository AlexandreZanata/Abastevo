-- EMP-TRIP-008. Missions: create, assign, accept and decline
-- (B-BR-EMP-353..358, EDR-228..231). A mission is planned work of one
-- organization: a unit, an optional ACTIVE main asset, a title and a planned
-- window. Its head row holds the current state, revision, assignee and
-- answer and changes only together with the insert-only transition of its
-- new revision. Company commands keep insert-only receipts; the assignee's
-- response arrives as the signed trip.mission.respond operation and is
-- published only with its ACCEPTED receipt. Nothing here starts a trip,
-- reserves an asset or touches a review case.
CREATE TABLE org_trip_missions(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 unit_id UUID NOT NULL,
 asset_id UUID,
 title TEXT NOT NULL CHECK(org_asset_text_valid(title,280,true)),
 starts_at TIMESTAMPTZ NOT NULL,
 ends_at TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL CHECK(state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','IN_PROGRESS','COMPLETED','CANCELLED')),
 revision BIGINT NOT NULL CHECK(revision>=1),
 assignee_employee_id UUID,
 assignee_membership_id UUID,
 answer TEXT CHECK(answer IS NULL OR answer IN('ACCEPT','DECLINE')),
 answer_reason TEXT CHECK(answer_reason IS NULL OR org_asset_text_valid(answer_reason,280,true)),
 created_by UUID NOT NULL REFERENCES accounts(id),
 created_by_membership_id UUID NOT NULL,
 create_operation_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_trip_missions_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_trip_missions_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_trip_missions_assignee_fkey FOREIGN KEY(org_id,assignee_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_missions_member_fkey FOREIGN KEY(org_id,assignee_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_missions_creator_fkey FOREIGN KEY(org_id,created_by_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_missions_window CHECK(isfinite(starts_at) AND isfinite(ends_at) AND ends_at>starts_at
  AND starts_at>'0001-01-01 00:00:00+00'::timestamptz AND ends_at<'10000-01-01 00:00:00+00'::timestamptz),
 CONSTRAINT org_trip_missions_assignee_pair CHECK((assignee_employee_id IS NULL)=(assignee_membership_id IS NULL)),
 -- The stored invariants of RestoreMission (EDR-228); the answer is always
 -- the current assignee's own.
 CONSTRAINT org_trip_missions_shape CHECK((state='DRAFT')=(revision=1)
  AND(answer IS NULL OR assignee_membership_id IS NOT NULL)
  AND((answer='DECLINE')=(answer_reason IS NOT NULL))
  AND CASE state
   WHEN 'DRAFT' THEN assignee_membership_id IS NULL AND answer IS NULL
   WHEN 'PLANNED' THEN assignee_membership_id IS NULL AND answer IS NULL
   WHEN 'ASSIGNED' THEN assignee_membership_id IS NOT NULL AND answer IS NULL
   WHEN 'ACCEPTED' THEN answer='ACCEPT'
   WHEN 'IN_PROGRESS' THEN answer='ACCEPT'
   WHEN 'COMPLETED' THEN answer='ACCEPT'
   WHEN 'DECLINED' THEN answer='DECLINE'
   ELSE true END)
);
CREATE INDEX org_trip_missions_assignee ON org_trip_missions(org_id,assignee_membership_id,id) WHERE assignee_membership_id IS NOT NULL;

-- One transition per revision: the full history survives reassignment,
-- decline and cancellation (BR-AUD-001).
CREATE TABLE org_trip_mission_transitions(
 org_id UUID NOT NULL,
 mission_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>=1),
 kind TEXT NOT NULL CHECK(kind IN('CREATE','ASSIGN','ACCEPT','DECLINE','CANCEL')),
 from_state TEXT CHECK(from_state IS NULL OR from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','IN_PROGRESS','COMPLETED','CANCELLED')),
 to_state TEXT NOT NULL CHECK(to_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','IN_PROGRESS','COMPLETED','CANCELLED')),
 source TEXT NOT NULL CHECK(source IN('COMPANY','DEVICE')),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),
 actor_membership_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 device_operation_id UUID GENERATED ALWAYS AS(CASE WHEN source='DEVICE' THEN operation_id END) STORED,
 enrollment_id UUID,
 assignee_employee_id UUID,
 assignee_membership_id UUID,
 reason TEXT CHECK(reason IS NULL OR org_asset_text_valid(reason,280,true)),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,mission_id,revision),
 CONSTRAINT org_trip_mission_transitions_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_mission_transitions_receipt_key UNIQUE(org_id,mission_id,revision,operation_id),
 CONSTRAINT org_trip_mission_transitions_mission_fkey FOREIGN KEY(org_id,mission_id) REFERENCES org_trip_missions(org_id,id),
 CONSTRAINT org_trip_mission_transitions_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_mission_transitions_operation_fkey FOREIGN KEY(org_id,device_operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_trip_mission_transitions_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_trip_mission_transitions_assignee_fkey FOREIGN KEY(org_id,assignee_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_mission_transitions_member_fkey FOREIGN KEY(org_id,assignee_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_mission_transitions_shape CHECK((assignee_employee_id IS NULL)=(assignee_membership_id IS NULL)
  AND((kind IN('ACCEPT','DECLINE'))=(source='DEVICE')) AND((source='DEVICE')=(enrollment_id IS NOT NULL))
  AND((kind IN('DECLINE','CANCEL'))=(reason IS NOT NULL))
  AND((kind='CREATE')=(from_state IS NULL)) AND(kind<>'CREATE' OR(revision=1 AND to_state='DRAFT'))
  AND(kind NOT IN('ASSIGN','ACCEPT','DECLINE') OR assignee_membership_id IS NOT NULL)
  AND(kind<>'CREATE' OR assignee_membership_id IS NULL)),
 CHECK(isfinite(recorded_at))
);

-- Insert-only receipts of the company commands (B-BR-EMP-358).
CREATE TABLE org_trip_mission_commands(
 org_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES accounts(id),
 mission_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('CREATE','ASSIGN','CANCEL')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>=1),
 result_state TEXT NOT NULL CHECK(result_state IN('DRAFT','ASSIGNED','CANCELLED')),
 warning_code TEXT CHECK(warning_code IS NULL OR warning_code IN('ASSET_BUSY','ASSET_UNAVAILABLE')),
 warning_availability TEXT CHECK(warning_availability IS NULL OR warning_availability IN('AVAILABLE','RESERVED','IN_USE','IN_MAINTENANCE','BLOCKED','UNAVAILABLE')),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_trip_mission_commands_transition_fkey FOREIGN KEY(org_id,mission_id,result_revision,operation_id)
  REFERENCES org_trip_mission_transitions(org_id,mission_id,revision,operation_id),
 CHECK((warning_code IS NULL)=(warning_availability IS NULL) AND(warning_code IS NULL OR kind='ASSIGN'))
);

-- The EDR-228 table: the next state of an open mission, or NULL.
CREATE FUNCTION org_trip_mission_next(from_state TEXT,command TEXT) RETURNS TEXT
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT CASE
  WHEN from_state IN('COMPLETED','CANCELLED','IN_PROGRESS') THEN NULL
  WHEN command='ASSIGN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'ASSIGNED'
  WHEN command='CANCEL' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'CANCELLED'
  WHEN command='ACCEPT' AND from_state='ASSIGNED' THEN 'ACCEPTED'
  WHEN command='DECLINE' AND from_state='ASSIGNED' THEN 'DECLINED'
 END
 $$;

-- A current company actor: the ACTIVE membership of the account.
CREATE FUNCTION org_trip_mission_actor_valid(p_org UUID,p_member UUID,p_account UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_memberships m WHERE m.org_id=p_org AND m.id=p_member AND m.account_id=p_account AND m.state='ACTIVE')
 $$;

-- An eligible assignee: an ACTIVE, unmerged employee whose live membership
-- is ACTIVE (the role coverage is the application's current authority).
CREATE FUNCTION org_trip_mission_assignee_valid(p_org UUID,p_employee UUID,p_member UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=p_org AND m.id=p_member AND m.employee_id=p_employee AND m.state='ACTIVE' AND m.account_id IS NOT NULL
  AND e.state='ACTIVE' AND e.merged_into IS NULL)
 $$;

-- B-BR-EMP-354: a new mission is DRAFT at revision 1 in an ACTIVE unit, with
-- an optional ACTIVE main asset, created by a current member.
CREATE FUNCTION org_trip_mission_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.state<>'DRAFT' OR NEW.revision<>1 OR NEW.assignee_membership_id IS NOT NULL OR NEW.answer IS NOT NULL
 OR NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.created_by_membership_id,NEW.created_by)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission source unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id AND u.state='ACTIVE')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission unit unavailable';END IF;
 IF NEW.asset_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id AND a.state='ACTIVE' AND a.kind<>'IMPLEMENT')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission asset unavailable';END IF;
 NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_source BEFORE INSERT ON org_trip_missions FOR EACH ROW EXECUTE FUNCTION org_trip_mission_source();

-- B-BR-EMP-353/358: one transition per revision, decided against the locked
-- head; a response is the exact signed operation of the current assignee.
CREATE FUNCTION org_trip_mission_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
  IF NEW.kind<>'ASSIGN' AND(NEW.assignee_employee_id IS DISTINCT FROM m.assignee_employee_id OR NEW.assignee_membership_id IS DISTINCT FROM m.assignee_membership_id)
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
CREATE TRIGGER org_trip_mission_transition_source BEFORE INSERT ON org_trip_mission_transitions FOR EACH ROW EXECUTE FUNCTION org_trip_mission_transition_source();

-- The head moves exactly to the already-inserted transition of its new
-- revision; identity, content and creation facts never change.
CREATE FUNCTION org_trip_mission_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
 OR t.kind='CREATE'
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_guard BEFORE UPDATE ON org_trip_missions FOR EACH ROW EXECUTE FUNCTION org_trip_mission_guard();

-- At commit: a mission has its CREATE transition; a transition is reached by
-- the head; a company transition has its receipt; a response is published
-- only with its ACCEPTED acknowledgement and effect at that revision.
CREATE FUNCTION org_trip_mission_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_mission_complete AFTER INSERT ON org_trip_missions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_mission_complete();
CREATE CONSTRAINT TRIGGER org_trip_mission_transition_complete AFTER INSERT ON org_trip_mission_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_mission_complete();

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trip_missions','org_trip_mission_transitions','org_trip_mission_commands'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 IF name='org_trip_missions' THEN
  EXECUTE format('CREATE TRIGGER %I BEFORE DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 ELSE
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 END IF;
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- The 223 asset-use source (every line kept) with the mission asset
-- recognised: a mission names only an ACTIVE main asset, never a
-- discardable draft, so the counts are unchanged (EDR-230).
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
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;

-- The 220 unit-use source (every line kept) with the mission unit recognised
-- as an association, never as current use (BR-AUTH-005, EDR-230).
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
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
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
