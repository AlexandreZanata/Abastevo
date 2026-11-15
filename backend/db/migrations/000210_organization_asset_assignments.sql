-- EMP-FLEET-006. Driver assignments: primary, additional and temporary
-- (B-BR-EMP-243..246, EDR-164..165). Additive: two new tables, their guards,
-- two same-transaction consumers of the employee termination and the asset
-- disposal, and the asset-use source with the new foreign key allow-listed.
-- Applied migrations stay unchanged; no existing row is written.

-- B-BR-EMP-243: an assignment is ENDED at an instant when its end is at or
-- before it, or when it was ended before its start (empty period).
CREATE FUNCTION org_asset_assignment_ended(valid_from TIMESTAMPTZ,valid_to TIMESTAMPTZ,selected TIMESTAMPTZ) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT valid_to IS NOT NULL AND(valid_to<=selected OR valid_to=valid_from)
$$;

-- One receipt per command (EDR-164): the created or ended identifiers in
-- command order and, for a replace, the ended primary.
CREATE TABLE org_asset_assignment_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL CHECK(asset_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('CREATE','END')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 assignment_ids UUID[] NOT NULL CHECK(cardinality(assignment_ids) BETWEEN 1 AND 100 AND array_position(assignment_ids,NULL) IS NULL AND array_ndims(assignment_ids)=1),
 replaced_id UUID CHECK(replaced_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_asset_assignment_commands_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_assignment_commands_result_check CHECK(kind='CREATE' OR(cardinality(assignment_ids)=1 AND replaced_id IS NULL))
);
CREATE TRIGGER org_asset_assignment_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_assignment_commands FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- B-BR-EMP-243/244: append-only rows with whole-microsecond half-open periods,
-- same-organization references and no overlapping PRIMARY per asset (the
-- empty range of an assignment ended before its start overlaps nothing).
CREATE TABLE org_asset_assignments(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL,employee_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('PRIMARY','ADDITIONAL','TEMPORARY')),
 valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,
 reason TEXT NOT NULL DEFAULT '' CHECK(reason='' OR org_asset_text_valid(reason,280,true)),
 operation_id UUID NOT NULL,
 assigned_by UUID NOT NULL CHECK(assigned_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 end_cause TEXT NOT NULL DEFAULT '' CHECK(end_cause IN('','ENDED','REPLACED','EMPLOYEE_TERMINATED','ASSET_DISPOSED')),
 ended_at TIMESTAMPTZ,end_operation_id UUID,
 ended_by UUID CHECK(ended_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 end_reason TEXT NOT NULL DEFAULT '' CHECK(end_reason='' OR org_asset_text_valid(end_reason,280,true)),
 source_revision BIGINT CHECK(source_revision>0),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_asset_assignments_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_assignments_employee_fkey FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_asset_assignments_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_asset_assignment_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_assignments_end_command_fkey FOREIGN KEY(org_id,end_operation_id) REFERENCES org_asset_assignment_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_assignments_bounds_check CHECK(isfinite(valid_from) AND valid_from>='0001-01-01 00:00:00+00'::timestamptz AND valid_from<'10000-01-01 00:00:00+00'::timestamptz
  AND date_trunc('microseconds',valid_from)=valid_from
  AND(valid_to IS NULL OR(isfinite(valid_to) AND valid_to>=valid_from AND valid_to<'10000-01-01 00:00:00+00'::timestamptz AND date_trunc('microseconds',valid_to)=valid_to))
  AND isfinite(recorded_at)),
 CONSTRAINT org_asset_assignments_period_check CHECK((kind<>'TEMPORARY' OR valid_to IS NOT NULL) AND(end_cause<>'' OR valid_to IS NULL OR valid_to>valid_from)),
 CONSTRAINT org_asset_assignments_end_check CHECK((end_cause='')=(ended_at IS NULL) AND(end_cause='' OR valid_to IS NOT NULL)
  AND(end_cause IN('ENDED','REPLACED'))=(end_operation_id IS NOT NULL AND ended_by IS NOT NULL)
  AND(end_cause IN('EMPLOYEE_TERMINATED','ASSET_DISPOSED'))=(source_revision IS NOT NULL)
  AND(end_cause IN('ENDED','REPLACED') OR(end_operation_id IS NULL AND ended_by IS NULL AND end_reason=''))),
 CONSTRAINT org_asset_assignments_primary_no_overlap EXCLUDE USING gist(org_id WITH =,asset_id WITH =,tstzrange(valid_from,valid_to,'[)') WITH &&) WHERE(kind='PRIMARY')
);
CREATE INDEX org_asset_assignments_asset_period ON org_asset_assignments(org_id,asset_id,valid_from,id);
CREATE INDEX org_asset_assignments_employee ON org_asset_assignments(org_id,employee_id,id);

ALTER TABLE org_asset_assignment_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_asset_assignment_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_asset_assignment_commands_scope ON org_asset_assignment_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_asset_assignment_commands FROM PUBLIC;
ALTER TABLE org_asset_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_asset_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY org_asset_assignments_scope ON org_asset_assignments USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_asset_assignments FROM PUBLIC;

-- Insert: under the organization root lock, an ACTIVE or INACTIVE asset and an
-- ACTIVE employee record of this organization, a start not before the
-- transaction, no end data; the recording instant is the server's. Update:
-- only ending an assignment that is not ENDED, moving its end earlier (never
-- later) but never before the transaction (except to the exact disposal
-- instant), with a cause proven against its source. Never delete or truncate.
CREATE FUNCTION org_asset_assignment_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE asset_state TEXT;asset_revision BIGINT;disposed TIMESTAMPTZ;employee_state TEXT;employee_revision BIGINT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept assignment changes';END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_assignments_asset_fkey',MESSAGE='private reference not found';END IF;
  SELECT state INTO employee_state FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_assignments_employee_fkey',MESSAGE='private reference not found';END IF;
  IF asset_state NOT IN('ACTIVE','INACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept assignment';END IF;
  IF employee_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee does not accept assignment';END IF;
  IF NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL
   OR NEW.valid_from<transaction_timestamp() THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial assignment';END IF;
  NEW.recorded_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.asset_id,NEW.employee_id,NEW.kind,NEW.valid_from,NEW.reason,NEW.operation_id,NEW.assigned_by,NEW.recorded_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.asset_id,OLD.employee_id,OLD.kind,OLD.valid_from,OLD.reason,OLD.operation_id,OLD.assigned_by,OLD.recorded_at)
  OR NEW.end_cause='' OR NEW.valid_to IS NULL OR NEW.valid_to<OLD.valid_from OR(OLD.valid_to IS NOT NULL AND NEW.valid_to>OLD.valid_to)
  OR public.org_asset_assignment_ended(OLD.valid_from,OLD.valid_to,transaction_timestamp()) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='assignment already ended or immutable';
 END IF;
 IF NEW.end_cause IN('ENDED','REPLACED') THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept assignment changes';END IF;
  IF NEW.valid_to<transaction_timestamp() OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment end source unavailable';
  END IF;
 ELSIF NEW.end_cause='EMPLOYEE_TERMINATED' THEN
  SELECT state,revision INTO employee_state,employee_revision FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
  IF employee_state IS DISTINCT FROM 'TERMINATED' OR employee_revision IS DISTINCT FROM NEW.source_revision OR NEW.valid_to<transaction_timestamp()
   OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment end source unavailable';
  END IF;
 ELSIF NEW.end_cause='ASSET_DISPOSED' THEN
  SELECT state,revision,disposed_at INTO asset_state,asset_revision,disposed FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF asset_state IS DISTINCT FROM 'DISPOSED' OR asset_revision IS DISTINCT FROM NEW.source_revision OR NEW.valid_to IS DISTINCT FROM GREATEST(OLD.valid_from,disposed)
   OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment end source unavailable';
  END IF;
 ELSE
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assignment end source unavailable';
 END IF;
 NEW.ended_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_assignment_write BEFORE INSERT OR UPDATE ON org_asset_assignments FOR EACH ROW EXECUTE FUNCTION org_asset_assignment_guard();
CREATE TRIGGER org_asset_assignments_immutable BEFORE DELETE OR TRUNCATE ON org_asset_assignments FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- B-BR-EMP-244, checked at commit: a REPLACED primary has its successor, a
-- PRIMARY of the same asset created by the ending command at its new end.
CREATE FUNCTION org_asset_assignment_replaced() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_asset_assignments s WHERE s.org_id=NEW.org_id AND s.asset_id=NEW.asset_id AND s.kind='PRIMARY'
  AND s.operation_id=NEW.end_operation_id AND s.valid_from=NEW.valid_to AND s.id<>NEW.id) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='replaced assignment has no successor';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_asset_assignment_replaced AFTER UPDATE ON org_asset_assignments DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW WHEN(NEW.end_cause='REPLACED') EXECUTE FUNCTION org_asset_assignment_replaced();

-- B-BR-EMP-245 (EDR-165): the same transaction that terminates an employee
-- record or disposes an asset ends every assignment that is not ENDED (a
-- primary replaced at a later start is not ENDED yet: its earlier end and
-- the system cause replace the command data).
CREATE FUNCTION org_asset_assignment_end_employee() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected TIMESTAMPTZ:=clock_timestamp();
BEGIN
 UPDATE public.org_asset_assignments SET valid_to=GREATEST(valid_from,selected),end_cause='EMPLOYEE_TERMINATED',source_revision=NEW.revision,end_operation_id=NULL,ended_by=NULL,end_reason=''
 WHERE org_id=NEW.org_id AND employee_id=NEW.id AND NOT public.org_asset_assignment_ended(valid_from,valid_to,selected);
 RETURN NULL;
END $$;
CREATE TRIGGER org_asset_assignment_end_employee AFTER UPDATE OF state ON org_employee_identities FOR EACH ROW
 WHEN(NEW.state='TERMINATED' AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_asset_assignment_end_employee();
CREATE FUNCTION org_asset_assignment_end_disposal() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected TIMESTAMPTZ:=clock_timestamp();
BEGIN
 UPDATE public.org_asset_assignments SET valid_to=GREATEST(valid_from,NEW.disposed_at),end_cause='ASSET_DISPOSED',source_revision=NEW.revision,end_operation_id=NULL,ended_by=NULL,end_reason=''
 WHERE org_id=NEW.org_id AND asset_id=NEW.id AND NOT public.org_asset_assignment_ended(valid_from,valid_to,selected);
 RETURN NULL;
END $$;
CREATE TRIGGER org_asset_assignment_end_disposal AFTER UPDATE OF state ON org_assets FOR EACH ROW
 WHEN(NEW.state='DISPOSED' AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_asset_assignment_end_disposal();

-- The 208 asset-use source: assignments name only assets that left DRAFT, so
-- they are never use of a discardable draft; the EMP-DOC-003 document-link key
-- is recognised when that table is installed (to_regclass keeps this body
-- valid with or without migration 000209).
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
 OR(c.conrelid='public.org_asset_assignments'::regclass AND c.conname='org_asset_assignments_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;
