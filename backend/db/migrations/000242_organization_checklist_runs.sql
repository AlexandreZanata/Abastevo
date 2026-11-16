-- EMP-CHECK-002. Checklist runs, their server recheck and the release of a
-- blocking run (B-BR-EMP-439..442, EDR-284/285). A run is the author's signed
-- check.run.complete: one immutable fact of one asset with its answers as
-- declared, the device outcome and the named policy revision, and the
-- server's recheck (outcome, policy revision in force at the declared
-- completion, superseded and disagreement flags), all re-proved here at
-- commit. The only change is the release of a BLOCKING run to
-- COMPLETED_WITH_ISSUES at revision 2, written by its release row. A run
-- holds no photo, amount or person's health. Nothing here is ever deleted.
CREATE TABLE org_checklist_runs(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 context TEXT NOT NULL CHECK(context IN('DEPARTURE','RETURN','AD_HOC')),
 template_id UUID NOT NULL,
 template_version INTEGER NOT NULL CHECK(template_version>0),
 policy_revision INTEGER NOT NULL CHECK(policy_revision>0),
 server_policy_revision INTEGER NOT NULL CHECK(server_policy_revision>0),
 started_at TIMESTAMPTZ NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 device_outcome TEXT NOT NULL CHECK(device_outcome IN('COMPLETED_OK','COMPLETED_WITH_ISSUES','BLOCKING')),
 recheck_outcome TEXT NOT NULL CHECK(recheck_outcome IN('COMPLETED_OK','COMPLETED_WITH_ISSUES','BLOCKING')),
 outcome TEXT NOT NULL CHECK(outcome IN('COMPLETED_OK','COMPLETED_WITH_ISSUES','BLOCKING')),
 outcome_differs BOOLEAN NOT NULL,
 template_superseded BOOLEAN NOT NULL,
 acknowledged BOOLEAN NOT NULL,
 author_account_id UUID NOT NULL,
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 revision BIGINT NOT NULL CHECK(revision BETWEEN 1 AND 2),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_checklist_runs_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_checklist_runs_template_key UNIQUE(org_id,id,template_id,template_version),
 CONSTRAINT org_checklist_runs_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_checklist_runs_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_checklist_runs_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_checklist_runs_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_checklist_runs_version_fkey FOREIGN KEY(org_id,template_id,template_version) REFERENCES org_checklist_template_versions(org_id,template_id,version),
 CONSTRAINT org_checklist_runs_policy_fkey FOREIGN KEY(org_id,template_id,policy_revision) REFERENCES org_checklist_policy_revisions(org_id,template_id,revision),
 CONSTRAINT org_checklist_runs_server_policy_fkey FOREIGN KEY(org_id,template_id,server_policy_revision) REFERENCES org_checklist_policy_revisions(org_id,template_id,revision),
 CONSTRAINT org_checklist_runs_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_checklist_runs_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_checklist_runs_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_checklist_runs_snapshot CHECK(cost_centre_id IS NULL OR unit_id IS NOT NULL),
 CONSTRAINT org_checklist_runs_differs CHECK(outcome_differs=(device_outcome<>recheck_outcome)),
 CONSTRAINT org_checklist_runs_acknowledged CHECK(NOT acknowledged OR device_outcome='COMPLETED_WITH_ISSUES'),
 CONSTRAINT org_checklist_runs_revision CHECK((revision=1 AND outcome=recheck_outcome) OR(revision=2 AND recheck_outcome='BLOCKING' AND outcome='COMPLETED_WITH_ISSUES')),
 CHECK(isfinite(started_at) AND started_at>='0001-01-01 00:00:00+00'::timestamptz AND started_at<=occurred_at AND date_trunc('microseconds',started_at)=started_at),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(updated_at))
);
CREATE INDEX org_checklist_runs_asset ON org_checklist_runs(org_id,asset_id,occurred_at,id);
CREATE INDEX org_checklist_runs_blocking ON org_checklist_runs(org_id,asset_id,occurred_at) WHERE recheck_outcome='BLOCKING';
CREATE INDEX org_checklist_runs_author ON org_checklist_runs(org_id,author_membership_id,id);

-- One answer per item of the named version, as declared (photos retained, EDR-286).
CREATE TABLE org_checklist_run_answers(
 org_id UUID NOT NULL,
 run_id UUID NOT NULL,
 template_id UUID NOT NULL,
 template_version INTEGER NOT NULL,
 position INTEGER NOT NULL CHECK(position BETWEEN 1 AND 40),
 answer TEXT NOT NULL CHECK(answer IN('OK','NOT_OK','NOT_APPLICABLE')),
 note TEXT CHECK(note IS NULL OR org_asset_text_valid(note,140,true)),
 photo BOOLEAN NOT NULL DEFAULT false CHECK(NOT photo),
 PRIMARY KEY(org_id,run_id,position),
 CONSTRAINT org_checklist_run_answers_run_fkey FOREIGN KEY(org_id,run_id,template_id,template_version) REFERENCES org_checklist_runs(org_id,id,template_id,template_version),
 CONSTRAINT org_checklist_run_answers_item_fkey FOREIGN KEY(org_id,template_id,template_version,position) REFERENCES org_checklist_template_items(org_id,template_id,version,position)
);

-- The release of a BLOCKING run (BR-CHECK-011); the row is the command receipt.
CREATE TABLE org_checklist_run_releases(
 org_id UUID NOT NULL,
 run_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision=2),
 reason TEXT NOT NULL CHECK(org_asset_text_valid(reason,500,true)),
 actor_account_id UUID NOT NULL,
 actor_membership_id UUID NOT NULL,
 actor_employee_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[0-9a-f]{64}$'),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,run_id),
 CONSTRAINT org_checklist_run_releases_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_checklist_run_releases_run_fkey FOREIGN KEY(org_id,run_id) REFERENCES org_checklist_runs(org_id,id),
 CONSTRAINT org_checklist_run_releases_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_checklist_run_releases_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_checklist_run_releases_actor_employee_fkey FOREIGN KEY(org_id,actor_employee_id) REFERENCES org_employee_identities(org_id,id),
 CHECK(isfinite(recorded_at))
);

-- The device operation of a run: exact type, entity, digest and an active
-- enrolled author whose account and employee record are the given ones.
CREATE FUNCTION org_checklist_run_operation_valid(p_org UUID,p_op UUID,p_run UUID,p_digest TEXT,p_member UUID,p_enrollment UUID,p_account UUID,p_employee UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=p_org AND o.operation_id=p_op AND o.operation_type='check.run.complete' AND o.operation_schema=1 AND o.entity_type='checklist_run'
  AND o.entity_id=p_run AND o.payload_digest=p_digest AND o.membership_id=p_member AND o.enrollment_id=p_enrollment AND o.owner_account_id=p_account
  AND m.employee_id=p_employee AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 $$;

-- B-BR-EMP-439..441 under the organization root and asset locks: the source,
-- a non-DRAFT asset, the allocation at the declared instant, the template of
-- the same context, revision 1 and the recheck outcome in force.
CREATE FUNCTION org_checklist_run_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;template_context TEXT;actual_unit UUID;actual_centre UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_checklist_run_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,NEW.author_account_id,NEW.author_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run source unavailable';END IF;
 SELECT * INTO a FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT context INTO template_context FROM public.org_checklist_templates WHERE org_id=NEW.org_id AND id=NEW.template_id;
 IF a.state='DRAFT' OR NEW.revision<>1 OR NEW.outcome<>NEW.recheck_outcome OR template_context IS DISTINCT FROM NEW.context
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid checklist run declaration';END IF;
 -- BR-AUTH-005: the FLEET-005 allocation at the declared instant.
 SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run allocation snapshot unavailable';END IF;
 NEW.received_at=clock_timestamp();NEW.updated_at=NEW.received_at;RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_run_source BEFORE INSERT ON org_checklist_runs FOR EACH ROW EXECUTE FUNCTION org_checklist_run_source();

-- Answers are written only with their run, before its acknowledgement exists.
CREATE FUNCTION org_checklist_run_answer_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_checklist_runs%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO r FROM public.org_checklist_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF r.revision<>1 OR EXISTS(SELECT 1 FROM public.org_sync_acknowledgements k WHERE k.org_id=r.org_id AND k.operation_id=r.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist run answers are immutable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_run_answer_source BEFORE INSERT ON org_checklist_run_answers FOR EACH ROW EXECUTE FUNCTION org_checklist_run_answer_source();

-- The only update: the release written by its row in this transaction;
-- every other column is immutable.
CREATE FUNCTION org_checklist_run_release_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF to_jsonb(NEW)-'outcome'-'revision'-'updated_at'<>to_jsonb(OLD)-'outcome'-'revision'-'updated_at' OR OLD.revision<>1 OR NEW.revision<>2
 OR OLD.outcome<>'BLOCKING' OR NEW.outcome<>'COMPLETED_WITH_ISSUES'
 OR NOT EXISTS(SELECT 1 FROM public.org_checklist_run_releases s WHERE s.org_id=OLD.org_id AND s.run_id=OLD.id AND s.revision=NEW.revision)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist run history is append-only';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_run_release_guard BEFORE UPDATE ON org_checklist_runs FOR EACH ROW EXECUTE FUNCTION org_checklist_run_release_guard();

-- B-BR-EMP-442: a BLOCKING run at revision 1 under the root and run locks,
-- an active actor and the separation of releaser and author (BR-CHECK-011).
CREATE FUNCTION org_checklist_run_release_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_checklist_runs%ROWTYPE;actor_person UUID;author_person UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO r FROM public.org_checklist_runs WHERE org_id=NEW.org_id AND id=NEW.run_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF r.outcome<>'BLOCKING' OR r.revision<>1 OR NEW.revision<>r.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid checklist run release';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.account_id=NEW.actor_account_id AND m.employee_id=NEW.actor_employee_id
  AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run releaser unavailable';END IF;
 actor_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.actor_employee_id);
 author_person=public.org_sync_review_employee_canonical(r.org_id,r.author_employee_id);
 IF actor_person IS NULL OR author_person IS NULL OR actor_person=author_person OR NEW.actor_account_id=r.author_account_id
 OR NEW.actor_membership_id=r.author_membership_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run self approval refused';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_run_release_source BEFORE INSERT ON org_checklist_run_releases FOR EACH ROW EXECUTE FUNCTION org_checklist_run_release_source();
CREATE FUNCTION org_checklist_run_release_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_checklist_runs SET outcome='COMPLETED_WITH_ISSUES',revision=NEW.revision WHERE org_id=NEW.org_id AND id=NEW.run_id AND outcome='BLOCKING' AND revision=NEW.revision-1;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid checklist run release';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_checklist_run_release_apply AFTER INSERT ON org_checklist_run_releases FOR EACH ROW EXECUTE FUNCTION org_checklist_run_release_apply();

-- At commit a run is published only with its ACCEPTED receipt and effect at
-- revision 1 and its database recheck (BR-CHECK-007): every item answered
-- once, the answer rule, the version's target covering the asset, the
-- policy revision in force at the declared completion (the first one before
-- it), the outcome and both flags. A release is published only with its own
-- audit event.
CREATE FUNCTION org_checklist_run_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;r public.org_checklist_runs%ROWTYPE;v public.org_checklist_template_versions%ROWTYPE;category UUID;
 answered INTEGER;issues INTEGER;critical INTEGER;refused BOOLEAN;in_force INTEGER;in_force_mode TEXT;computed TEXT;superseded BOOLEAN;archived TIMESTAMPTZ;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_checklist_runs' THEN
  SELECT * INTO r FROM public.org_checklist_runs WHERE org_id=NEW.org_id AND id=NEW.id AND operation_id=NEW.operation_id;
  IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements k JOIN public.org_sync_effects e USING(org_id,operation_id)
   WHERE k.org_id=NEW.org_id AND k.operation_id=NEW.operation_id AND k.result='ACCEPTED' AND k.entity_revision=1 AND e.entity_revision=1)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run publication unavailable';END IF;
  SELECT * INTO v FROM public.org_checklist_template_versions WHERE org_id=r.org_id AND template_id=r.template_id AND version=r.template_version;
  SELECT category_id INTO category FROM public.org_assets WHERE org_id=r.org_id AND id=r.asset_id;
  SELECT count(*),count(*) FILTER(WHERE a.answer='NOT_OK'),count(*) FILTER(WHERE a.answer='NOT_OK' AND i.critical),
   COALESCE(bool_or((a.answer='NOT_APPLICABLE' AND NOT i.optional) OR(a.answer='NOT_OK' AND i.photo_on_not_ok AND NOT a.photo)),false)
  INTO answered,issues,critical,refused
  FROM public.org_checklist_run_answers a JOIN public.org_checklist_template_items i ON(i.org_id,i.template_id,i.version,i.position)=(a.org_id,a.template_id,a.template_version,a.position)
  WHERE a.org_id=r.org_id AND a.run_id=r.id;
  SELECT p.revision,p.mode INTO in_force,in_force_mode FROM public.org_checklist_policy_revisions p
  WHERE p.org_id=r.org_id AND p.template_id=r.template_id AND p.effective_at<=r.occurred_at ORDER BY p.revision DESC LIMIT 1;
  IF in_force IS NULL THEN
   SELECT p.revision,p.mode INTO in_force,in_force_mode FROM public.org_checklist_policy_revisions p WHERE p.org_id=r.org_id AND p.template_id=r.template_id AND p.revision=1;
  END IF;
  computed=CASE WHEN issues=0 THEN 'COMPLETED_OK' WHEN critical>0 AND in_force_mode='BLOCK' THEN 'BLOCKING' ELSE 'COMPLETED_WITH_ISSUES' END;
  SELECT archived_at INTO archived FROM public.org_checklist_templates WHERE org_id=r.org_id AND id=r.template_id;
  superseded=(archived IS NOT NULL AND archived<=r.occurred_at) OR EXISTS(SELECT 1 FROM public.org_checklist_template_versions w
   WHERE w.org_id=r.org_id AND w.template_id=r.template_id AND w.version>r.template_version AND w.published_at<=r.occurred_at);
  IF answered<>v.item_count OR refused OR r.recheck_outcome<>computed OR r.server_policy_revision<>in_force OR r.template_superseded<>superseded
  OR NOT(v.target_kind='ORGANIZATION' OR EXISTS(SELECT 1 FROM public.org_checklist_template_targets t WHERE t.org_id=r.org_id AND t.template_id=r.template_id
   AND t.version=r.template_version AND(t.asset_id=r.asset_id OR t.category_id=category)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run recheck refused';END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id
  AND a.target_type='checklist_run' AND a.target_id=NEW.run_id AND a.request_hash=NEW.request_hash AND a.action='check.run.released')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist run release publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_checklist_run_published AFTER INSERT ON org_checklist_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_checklist_run_published();
CREATE CONSTRAINT TRIGGER org_checklist_run_release_published AFTER INSERT ON org_checklist_run_releases DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_checklist_run_published();

CREATE FUNCTION org_checklist_run_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist run history is append-only';END $$;
CREATE TRIGGER org_checklist_runs_retained BEFORE DELETE OR TRUNCATE ON org_checklist_runs FOR EACH STATEMENT EXECUTE FUNCTION org_checklist_run_keep_history();
CREATE TRIGGER org_checklist_run_answers_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_checklist_run_answers FOR EACH STATEMENT EXECUTE FUNCTION org_checklist_run_keep_history();
CREATE TRIGGER org_checklist_run_releases_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_checklist_run_releases FOR EACH STATEMENT EXECUTE FUNCTION org_checklist_run_keep_history();
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_checklist_runs','org_checklist_run_answers','org_checklist_run_releases'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- The 232 asset-use source (every line kept) with the run key recognised as an
-- association: a run never names a DRAFT asset (the source trigger), so it
-- cannot block a discard, and it does not fix the asset kind.
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
 OR(c.conrelid='public.org_checklist_runs'::regclass AND c.conname='org_checklist_runs_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;

-- The 231 unit-use source (every line kept) with the run's historical snapshot
-- recognised as an association, never as current use.
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
 OR(c.conrelid='public.org_checklist_runs'::regclass AND c.conname='org_checklist_runs_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
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

-- The 231 cost-centre-use source (every line kept) with the run's historical
-- snapshot recognised as an association, never as current use.
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
   OR(c.conrelid='public.org_checklist_runs'::regclass AND c.conname='org_checklist_runs_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
