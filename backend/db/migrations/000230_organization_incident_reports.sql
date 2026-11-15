-- EMP-CHECK-003. Damage and incident reports (B-BR-EMP-369..374,
-- EDR-238..241). A report is the reporter's signed check.incident.report: one
-- immutable fact of one asset (and optionally of the reporter's trip that
-- contains its instant) with the unit and cost centre the asset had at the
-- declared instant. Its handling is an append-only history; the report row
-- changes only its state, revision and update instant, one step at a time,
-- each backed by its history row. A report holds no amount, cost, document
-- byte, file name or person's health; its photos are EMP-DOC-003 links with
-- the new target kind INCIDENT. Nothing here is ever deleted.
CREATE TABLE org_incident_reports(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 report_type TEXT NOT NULL CHECK(report_type IN('DAMAGE','INCIDENT')),
 incident_kind TEXT CHECK(incident_kind IN('BREAKDOWN','ACCIDENT','THEFT','FINE_NOTICE','OTHER')),
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 trip_id UUID,
 description TEXT NOT NULL CHECK(org_asset_text_valid(description,1000,true)),
 location TEXT CHECK(location IS NULL OR org_asset_text_valid(location,140,true)),
 blocks_use BOOLEAN NOT NULL,
 reporter_account_id UUID NOT NULL,
 reporter_membership_id UUID NOT NULL,
 reporter_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 state TEXT NOT NULL CHECK(state IN('REPORTED','ACKNOWLEDGED','IN_TREATMENT','RESOLVED','DISMISSED')),
 revision BIGINT NOT NULL CHECK(revision BETWEEN 1 AND 4),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_incident_reports_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_incident_reports_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_incident_reports_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_incident_reports_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_incident_reports_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_incident_reports_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_incident_reports_reporter_fkey FOREIGN KEY(org_id,reporter_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_incident_reports_reporter_employee_fkey FOREIGN KEY(org_id,reporter_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_incident_reports_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_incident_reports_kind CHECK((report_type='INCIDENT')=(incident_kind IS NOT NULL)),
 CONSTRAINT org_incident_reports_snapshot CHECK(cost_centre_id IS NULL OR unit_id IS NOT NULL),
 CONSTRAINT org_incident_reports_revision CHECK((state='REPORTED')=(revision=1)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(updated_at))
);
CREATE INDEX org_incident_reports_trip ON org_incident_reports(org_id,trip_id,occurred_at,occurred_submicro,id) WHERE trip_id IS NOT NULL;
CREATE INDEX org_incident_reports_reporter ON org_incident_reports(org_id,reporter_membership_id,id);
CREATE INDEX org_incident_reports_asset ON org_incident_reports(org_id,asset_id,id);

-- The handling history (UC-CHECK-04); each row is also the command receipt.
CREATE TABLE org_incident_report_transitions(
 org_id UUID NOT NULL,
 report_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision BETWEEN 2 AND 4),
 action TEXT NOT NULL CHECK(action IN('ACKNOWLEDGE','TREAT','RESOLVE','DISMISS')),
 from_state TEXT NOT NULL CHECK(from_state IN('REPORTED','ACKNOWLEDGED','IN_TREATMENT')),
 to_state TEXT NOT NULL CHECK(to_state IN('ACKNOWLEDGED','IN_TREATMENT','RESOLVED','DISMISSED')),
 note TEXT,
 duplicate_of UUID,
 actor_account_id UUID NOT NULL,
 actor_membership_id UUID NOT NULL,
 actor_employee_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[0-9a-f]{64}$'),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,report_id,revision),
 CONSTRAINT org_incident_report_transitions_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_incident_report_transitions_report_fkey FOREIGN KEY(org_id,report_id) REFERENCES org_incident_reports(org_id,id),
 CONSTRAINT org_incident_report_transitions_duplicate_fkey FOREIGN KEY(org_id,duplicate_of) REFERENCES org_incident_reports(org_id,id),
 CONSTRAINT org_incident_report_transitions_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_incident_report_transitions_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_incident_report_transitions_actor_employee_fkey FOREIGN KEY(org_id,actor_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_incident_report_transitions_step CHECK((action='ACKNOWLEDGE' AND from_state='REPORTED' AND to_state='ACKNOWLEDGED')
  OR(action='TREAT' AND from_state IN('REPORTED','ACKNOWLEDGED') AND to_state='IN_TREATMENT')
  OR(action='RESOLVE' AND to_state='RESOLVED') OR(action='DISMISS' AND to_state='DISMISSED')),
 -- BR-CHECK-017: an acknowledgement has no note; the stated action, the
 -- resolution note and the dismissal reason are required (1..500 scalars).
 CONSTRAINT org_incident_report_transitions_note CHECK(CASE WHEN action='ACKNOWLEDGE' THEN note IS NULL ELSE note IS NOT NULL AND org_asset_text_valid(note,500,true) END),
 CONSTRAINT org_incident_report_transitions_duplicate CHECK(duplicate_of IS NULL OR(action='DISMISS' AND duplicate_of<>report_id)),
 CHECK(isfinite(recorded_at))
);

-- The device operation of a report: exact type, entity, digest and an
-- active enrolled reporter whose account and employee record are the given ones.
CREATE FUNCTION org_incident_report_operation_valid(p_org UUID,p_op UUID,p_report UUID,p_digest TEXT,p_member UUID,p_enrollment UUID,p_account UUID,p_employee UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=p_org AND o.operation_id=p_op AND o.operation_type='check.incident.report' AND o.operation_schema=1 AND o.entity_type='incident_report'
  AND o.entity_id=p_report AND o.payload_digest=p_digest AND o.membership_id=p_member AND o.enrollment_id=p_enrollment AND o.owner_account_id=p_account
  AND m.employee_id=p_employee AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 $$;

-- B-BR-EMP-369..371 under the organization root, asset and trip locks: the
-- source, a non-DRAFT asset, the allocation at the declared instant and, for
-- a trip, the same asset, the trip's author and its time window.
CREATE FUNCTION org_incident_report_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;actual_unit UUID;actual_centre UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_incident_report_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.reporter_membership_id,NEW.enrollment_id,NEW.reporter_account_id,NEW.reporter_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report source unavailable';END IF;
 SELECT * INTO a FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF a.state='DRAFT' OR NEW.state<>'REPORTED' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept a report';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid incident report declaration';END IF;
 -- BR-AUTH-005: the FLEET-005 allocation at the declared instant.
 SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report allocation snapshot unavailable';END IF;
 IF NEW.trip_id IS NOT NULL THEN
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
  IF t.asset_id<>NEW.asset_id OR t.author_membership_id<>NEW.reporter_membership_id OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(f.trip_id IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report trip unavailable';END IF;
 END IF;
 NEW.received_at=clock_timestamp();NEW.updated_at=NEW.received_at;RETURN NEW;
END $$;
CREATE TRIGGER org_incident_report_source BEFORE INSERT ON org_incident_reports FOR EACH ROW EXECUTE FUNCTION org_incident_report_source();

-- The only update: the next revision written by its history row in this
-- transaction; every other column is immutable.
CREATE FUNCTION org_incident_report_step_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF to_jsonb(NEW)-'state'-'revision'-'updated_at'<>to_jsonb(OLD)-'state'-'revision'-'updated_at' OR NEW.revision<>OLD.revision+1
 OR NOT EXISTS(SELECT 1 FROM public.org_incident_report_transitions s WHERE s.org_id=OLD.org_id AND s.report_id=OLD.id AND s.revision=NEW.revision
  AND s.from_state=OLD.state AND s.to_state=NEW.state)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='incident report history is append-only';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_incident_report_step_guard BEFORE UPDATE ON org_incident_reports FOR EACH ROW EXECUTE FUNCTION org_incident_report_step_guard();

-- B-BR-EMP-373: the step from the current state and revision under the root
-- and report locks, an active handler, a duplicate of the same asset and the
-- separation of handler and reporter for a blocking report (BR-CHECK-011).
CREATE FUNCTION org_incident_report_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_incident_reports%ROWTYPE;actor_person UUID;reporter_person UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO r FROM public.org_incident_reports WHERE org_id=NEW.org_id AND id=NEW.report_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.from_state<>r.state OR NEW.revision<>r.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid incident report transition';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.account_id=NEW.actor_account_id AND m.employee_id=NEW.actor_employee_id
  AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report handler unavailable';END IF;
 IF NEW.duplicate_of IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_incident_reports d WHERE d.org_id=NEW.org_id AND d.id=NEW.duplicate_of AND d.asset_id=r.asset_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report duplicate unavailable';END IF;
 IF r.blocks_use AND NEW.action IN('RESOLVE','DISMISS') THEN
  actor_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.actor_employee_id);
  reporter_person=public.org_sync_review_employee_canonical(r.org_id,r.reporter_employee_id);
  IF actor_person IS NULL OR reporter_person IS NULL OR actor_person=reporter_person OR NEW.actor_account_id=r.reporter_account_id
  OR NEW.actor_membership_id=r.reporter_membership_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report self approval refused';END IF;
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_incident_report_transition_source BEFORE INSERT ON org_incident_report_transitions FOR EACH ROW EXECUTE FUNCTION org_incident_report_transition_source();
CREATE FUNCTION org_incident_report_transition_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_incident_reports SET state=NEW.to_state,revision=NEW.revision WHERE org_id=NEW.org_id AND id=NEW.report_id AND state=NEW.from_state AND revision=NEW.revision-1;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid incident report transition';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_incident_report_transition_apply AFTER INSERT ON org_incident_report_transitions FOR EACH ROW EXECUTE FUNCTION org_incident_report_transition_apply();

-- At commit a report is published only with its ACCEPTED receipt and effect
-- at revision 1, and a step only with its own audit event.
CREATE FUNCTION org_incident_report_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_incident_reports' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_incident_reports r WHERE r.org_id=NEW.org_id AND r.id=NEW.id AND r.operation_id=NEW.operation_id)
  OR NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a JOIN public.org_sync_effects e USING(org_id,operation_id)
   WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result='ACCEPTED' AND a.entity_revision=1 AND e.entity_revision=1)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report publication unavailable';END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id
  AND a.target_type='incident_report' AND a.target_id=NEW.report_id AND a.request_hash=NEW.request_hash
  AND a.action=CASE NEW.action WHEN 'ACKNOWLEDGE' THEN 'check.report.acknowledged' WHEN 'TREAT' THEN 'check.report.treated' WHEN 'RESOLVE' THEN 'check.report.resolved' ELSE 'check.report.dismissed' END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report step publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_incident_report_published AFTER INSERT ON org_incident_reports DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_incident_report_published();
CREATE CONSTRAINT TRIGGER org_incident_report_transition_published AFTER INSERT ON org_incident_report_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_incident_report_published();

CREATE FUNCTION org_incident_report_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='incident report history is append-only';END $$;
CREATE TRIGGER org_incident_reports_retained BEFORE DELETE OR TRUNCATE ON org_incident_reports FOR EACH STATEMENT EXECUTE FUNCTION org_incident_report_keep_history();
CREATE TRIGGER org_incident_report_transitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_incident_report_transitions FOR EACH STATEMENT EXECUTE FUNCTION org_incident_report_keep_history();
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_incident_reports','org_incident_report_transitions'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- EDR-239: photos are EMP-DOC-003 links with the target kind INCIDENT,
-- carried by their own typed same-organization reference.
ALTER TABLE org_document_links ADD COLUMN incident_report_id UUID;
ALTER TABLE org_document_links ADD CONSTRAINT org_document_links_incident_fkey FOREIGN KEY(org_id,incident_report_id) REFERENCES org_incident_reports(org_id,id),
 DROP CONSTRAINT org_document_links_target_kind_check,
 ADD CONSTRAINT org_document_links_target_kind_check CHECK(target_kind IN('ASSET','EMPLOYEE','METER_READING','INCIDENT')),
 DROP CONSTRAINT org_document_links_binding,
 ADD CONSTRAINT org_document_links_binding CHECK(CASE target_kind
  WHEN 'ASSET' THEN asset_id IS NOT NULL AND asset_id=target_id AND num_nonnulls(employee_id,reading_id,held_reading_id,incident_report_id)=0
  WHEN 'EMPLOYEE' THEN employee_id IS NOT NULL AND employee_id=target_id AND num_nonnulls(asset_id,reading_id,held_reading_id,incident_report_id)=0
  WHEN 'METER_READING' THEN num_nonnulls(asset_id,employee_id,incident_report_id)=0 AND num_nonnulls(reading_id,held_reading_id)=1
   AND COALESCE(reading_id,held_reading_id)=target_id
  WHEN 'INCIDENT' THEN incident_report_id IS NOT NULL AND incident_report_id=target_id AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id)=0
  ELSE false END);

-- The 223 asset-use source with the report key recognised as an
-- association: a report never names a DRAFT asset (the source trigger), so
-- it cannot block a discard, and it does not fix the asset kind.
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
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;

-- The 220 unit-use source with the report's historical snapshot recognised
-- as an association, never as current use.
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
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
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

-- The 220 cost-centre-use source with the report's historical snapshot
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
   OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_cost_centre_fkey')
   OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_cost_centre_fkey')
   OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
