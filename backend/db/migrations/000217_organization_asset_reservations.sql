-- EMP-FLEET-007. Reservations, blocks and derived availability
-- (B-BR-EMP-294..301, EDR-192..195). Additive: three new tables, their
-- guards, one same-transaction consumer of an asset leaving service, and the
-- asset-use source with the two new foreign keys allow-listed. Availability
-- itself is never stored. Applied migrations stay unchanged; no existing row
-- is written.

-- B-BR-EMP-296: a CONFIRMED reservation no longer holds the asset from the
-- earlier of its start plus the no-show tolerance (seconds) and its end.
CREATE FUNCTION org_asset_reservation_deadline(starts_at TIMESTAMPTZ,ends_at TIMESTAMPTZ,tolerance INTEGER) RETURNS TIMESTAMPTZ
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT LEAST(starts_at+make_interval(secs=>tolerance),ends_at)
$$;

-- One receipt per command (EDR-192): the reservation or block it acted on
-- and, for reservation commands, the state it left.
CREATE TABLE org_asset_reservation_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL CHECK(asset_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('REQUEST','CONFIRM','CANCEL','BLOCK','UNBLOCK')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 target_id UUID NOT NULL CHECK(target_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 result_state TEXT NOT NULL DEFAULT '' CHECK(result_state IN('','REQUESTED','CONFIRMED','CANCELLED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_asset_reservation_commands_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_reservation_commands_result_check CHECK((kind IN('BLOCK','UNBLOCK'))=(result_state='')
  AND(kind<>'REQUEST' OR result_state IN('REQUESTED','CONFIRMED')) AND(kind<>'CONFIRM' OR result_state='CONFIRMED') AND(kind<>'CANCEL' OR result_state='CANCELLED'))
);
CREATE TRIGGER org_asset_reservation_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_reservation_commands FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- B-BR-EMP-294/295: one row per reservation; the stored state moves only by
-- the guarded §6.4 transitions and two CONFIRMED (or FULFILLED) windows of
-- one asset never overlap.
CREATE TABLE org_asset_reservations(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL,employee_id UUID,
 purpose TEXT NOT NULL CHECK(org_asset_text_valid(purpose,280,true)),
 starts_at TIMESTAMPTZ NOT NULL,ends_at TIMESTAMPTZ NOT NULL,
 no_show_tolerance INTEGER NOT NULL CHECK(no_show_tolerance BETWEEN 900 AND 14400),
 state TEXT NOT NULL CHECK(state IN('REQUESTED','CONFIRMED','FULFILLED','CANCELLED','NO_SHOW','EXPIRED')),
 revision BIGINT NOT NULL CHECK(revision>0),
 operation_id UUID NOT NULL,
 requested_by UUID NOT NULL CHECK(requested_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 requested_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 confirm_operation_id UUID,
 confirmed_by UUID CHECK(confirmed_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 confirmed_at TIMESTAMPTZ,
 end_cause TEXT NOT NULL DEFAULT '' CHECK(end_cause IN('','CANCELLED','ASSET_LEFT_SERVICE','TIME')),
 ended_at TIMESTAMPTZ,end_operation_id UUID,
 ended_by UUID CHECK(ended_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 end_reason TEXT NOT NULL DEFAULT '' CHECK(end_reason='' OR org_asset_text_valid(end_reason,280,true)),
 source_revision BIGINT CHECK(source_revision>0),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_asset_reservations_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_reservations_employee_fkey FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_asset_reservations_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_asset_reservation_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_reservations_confirm_command_fkey FOREIGN KEY(org_id,confirm_operation_id) REFERENCES org_asset_reservation_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_reservations_end_command_fkey FOREIGN KEY(org_id,end_operation_id) REFERENCES org_asset_reservation_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_reservations_bounds_check CHECK(isfinite(starts_at) AND starts_at>='0001-01-01 00:00:00+00'::timestamptz AND date_trunc('microseconds',starts_at)=starts_at
  AND isfinite(ends_at) AND ends_at>starts_at AND ends_at<'10000-01-01 00:00:00+00'::timestamptz AND date_trunc('microseconds',ends_at)=ends_at
  AND isfinite(requested_at) AND(confirmed_at IS NULL OR isfinite(confirmed_at)) AND(ended_at IS NULL OR isfinite(ended_at))),
 CONSTRAINT org_asset_reservations_confirm_check CHECK((confirm_operation_id IS NULL)=(confirmed_by IS NULL) AND(confirmed_by IS NULL)=(confirmed_at IS NULL)
  AND(state NOT IN('REQUESTED','EXPIRED') OR confirmed_at IS NULL) AND(state NOT IN('CONFIRMED','NO_SHOW','FULFILLED') OR confirmed_at IS NOT NULL)),
 CONSTRAINT org_asset_reservations_end_check CHECK((end_cause='')=(ended_at IS NULL)
  AND(state IN('REQUESTED','CONFIRMED','FULFILLED'))=(end_cause='')
  AND(state<>'CANCELLED' OR end_cause IN('CANCELLED','ASSET_LEFT_SERVICE')) AND(state NOT IN('EXPIRED','NO_SHOW') OR end_cause='TIME')
  AND(end_cause='CANCELLED')=(end_operation_id IS NOT NULL AND ended_by IS NOT NULL)
  AND(end_cause='ASSET_LEFT_SERVICE')=(source_revision IS NOT NULL)
  AND(end_cause='CANCELLED' OR(end_operation_id IS NULL AND ended_by IS NULL AND end_reason=''))),
 CONSTRAINT org_asset_reservations_confirmed_no_overlap EXCLUDE USING gist(org_id WITH =,asset_id WITH =,tstzrange(starts_at,ends_at,'[)') WITH &&) WHERE(state IN('CONFIRMED','FULFILLED'))
);
CREATE INDEX org_asset_reservations_asset_window ON org_asset_reservations(org_id,asset_id,starts_at,id);
CREATE INDEX org_asset_reservations_employee ON org_asset_reservations(org_id,employee_id,id) WHERE employee_id IS NOT NULL;

-- B-BR-EMP-298: append-only blocks with whole-microsecond half-open periods
-- (open-ended allowed); removal sets the end and is the only change.
CREATE TABLE org_asset_blocks(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL,
 reason_kind TEXT NOT NULL CHECK(reason_kind IN('ADMINISTRATIVE','SAFETY','DOCUMENT','INVESTIGATION','LENT_OUT')),
 note TEXT NOT NULL DEFAULT '' CHECK(note='' OR org_asset_text_valid(note,280,true)),
 valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,
 operation_id UUID NOT NULL,
 blocked_by UUID NOT NULL CHECK(blocked_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 removed_at TIMESTAMPTZ,remove_operation_id UUID,
 removed_by UUID CHECK(removed_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 removal_reason TEXT NOT NULL DEFAULT '' CHECK(removal_reason='' OR org_asset_text_valid(removal_reason,280,true)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_asset_blocks_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_blocks_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_asset_reservation_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_blocks_remove_command_fkey FOREIGN KEY(org_id,remove_operation_id) REFERENCES org_asset_reservation_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_blocks_bounds_check CHECK(isfinite(valid_from) AND valid_from>='0001-01-01 00:00:00+00'::timestamptz AND valid_from<'10000-01-01 00:00:00+00'::timestamptz
  AND date_trunc('microseconds',valid_from)=valid_from
  AND(valid_to IS NULL OR(isfinite(valid_to) AND valid_to>=valid_from AND valid_to<'10000-01-01 00:00:00+00'::timestamptz AND date_trunc('microseconds',valid_to)=valid_to))
  AND isfinite(recorded_at) AND(removed_at IS NULL OR isfinite(removed_at))),
 CONSTRAINT org_asset_blocks_period_check CHECK(removed_at IS NOT NULL OR valid_to IS NULL OR valid_to>valid_from),
 CONSTRAINT org_asset_blocks_removal_check CHECK((removed_at IS NULL)=(remove_operation_id IS NULL) AND(removed_at IS NULL)=(removed_by IS NULL)
  AND(removed_at IS NULL)=(removal_reason='') AND(removed_at IS NULL OR valid_to IS NOT NULL))
);
CREATE INDEX org_asset_blocks_asset_period ON org_asset_blocks(org_id,asset_id,valid_from,id);

DO $$
DECLARE name TEXT;
BEGIN
 FOREACH name IN ARRAY ARRAY['org_asset_reservation_commands','org_asset_reservations','org_asset_blocks'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- B-BR-EMP-295: an unremoved block whose period overlaps a window refuses
-- its confirmation (the window alone is reported by the application).
CREATE FUNCTION org_asset_reservation_blocked(selected_org UUID,selected_asset UUID,starts_at TIMESTAMPTZ,ends_at TIMESTAMPTZ) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_asset_blocks b WHERE b.org_id=selected_org AND b.asset_id=selected_asset
  AND tstzrange(b.valid_from,b.valid_to,'[)')&&tstzrange(starts_at,ends_at,'[)'))
$$;

-- Insert: under the organization root lock, an ACTIVE asset and, when named,
-- an ACTIVE employee record of this organization; a window starting after
-- the transaction; REQUESTED, or CONFIRMED by its own requesting command
-- outside every block. Update: only the §6.4 rows (FULFILLED has no source
-- before EMP-TRIP-023), each proven against its cause and the clock. Never
-- delete or truncate.
CREATE FUNCTION org_asset_reservation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE asset_state TEXT;asset_revision BIGINT;employee_state TEXT;now_at TIMESTAMPTZ:=transaction_timestamp();
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_reservations_asset_fkey',MESSAGE='private reference not found';END IF;
  IF NEW.employee_id IS NOT NULL THEN
   SELECT state INTO employee_state FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
   IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_reservations_employee_fkey',MESSAGE='private reference not found';END IF;
   IF employee_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee does not accept reservation';END IF;
  END IF;
  IF asset_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept reservation';END IF;
  IF NEW.state NOT IN('REQUESTED','CONFIRMED') OR NEW.revision<>1 OR NEW.starts_at<=now_at
   OR NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL
   OR(NEW.state='REQUESTED' AND(NEW.confirm_operation_id IS NOT NULL OR NEW.confirmed_by IS NOT NULL))
   OR(NEW.state='CONFIRMED' AND(NEW.confirm_operation_id IS DISTINCT FROM NEW.operation_id OR NEW.confirmed_by IS DISTINCT FROM NEW.requested_by)) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial reservation';
  END IF;
  IF NEW.state='CONFIRMED' AND public.org_asset_reservation_blocked(NEW.org_id,NEW.asset_id,NEW.starts_at,NEW.ends_at) THEN
   RAISE EXCEPTION USING ERRCODE='23P01',CONSTRAINT='org_asset_reservations_block_overlap',MESSAGE='reservation window conflicts';
  END IF;
  NEW.requested_at=clock_timestamp();
  NEW.confirmed_at=CASE WHEN NEW.state='CONFIRMED' THEN NEW.requested_at END;
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.asset_id,NEW.employee_id,NEW.purpose,NEW.starts_at,NEW.ends_at,NEW.no_show_tolerance,NEW.operation_id,NEW.requested_by,NEW.requested_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.asset_id,OLD.employee_id,OLD.purpose,OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance,OLD.operation_id,OLD.requested_by,OLD.requested_at)
  OR NEW.revision IS DISTINCT FROM OLD.revision+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation is immutable';
 END IF;
 IF OLD.state='REQUESTED' AND NEW.state='CONFIRMED' THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
  IF now_at>=OLD.starts_at OR NEW.confirm_operation_id IS NULL OR NEW.confirmed_by IS NULL OR NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL
   OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation transition unavailable';
  END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF asset_state IS DISTINCT FROM 'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept reservation';END IF;
  IF public.org_asset_reservation_blocked(NEW.org_id,NEW.asset_id,NEW.starts_at,NEW.ends_at) THEN
   RAISE EXCEPTION USING ERRCODE='23P01',CONSTRAINT='org_asset_reservations_block_overlap',MESSAGE='reservation window conflicts';
  END IF;
  NEW.confirmed_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.confirm_operation_id,NEW.confirmed_by,NEW.confirmed_at) IS DISTINCT FROM ROW(OLD.confirm_operation_id,OLD.confirmed_by,OLD.confirmed_at) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation is immutable';
 END IF;
 IF OLD.state IN('REQUESTED','CONFIRMED') AND NEW.state='CANCELLED' THEN
  IF OLD.state='REQUESTED' AND now_at>=OLD.starts_at OR OLD.state='CONFIRMED' AND now_at>=public.org_asset_reservation_deadline(OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance) THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation already ended';
  END IF;
  IF NEW.end_cause='CANCELLED' THEN
   PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
   IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
   IF NEW.end_operation_id IS NULL OR NEW.ended_by IS NULL OR NEW.source_revision IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
   END IF;
  ELSIF NEW.end_cause='ASSET_LEFT_SERVICE' THEN
   SELECT state,revision INTO asset_state,asset_revision FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
   IF asset_state NOT IN('INACTIVE','DISPOSED','ARCHIVED') OR asset_revision IS DISTINCT FROM NEW.source_revision
    OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
   END IF;
  ELSE
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
  END IF;
  NEW.ended_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF(OLD.state='REQUESTED' AND NEW.state='EXPIRED' AND now_at>=OLD.starts_at)
  OR(OLD.state='CONFIRMED' AND NEW.state='NO_SHOW' AND now_at>=public.org_asset_reservation_deadline(OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance)) THEN
  IF NEW.end_cause<>'TIME' OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
  END IF;
  NEW.ended_at=clock_timestamp();
  RETURN NEW;
 END IF;
 RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation transition unavailable';
END $$;
CREATE TRIGGER org_asset_reservation_write BEFORE INSERT OR UPDATE ON org_asset_reservations FOR EACH ROW EXECUTE FUNCTION org_asset_reservation_guard();
CREATE TRIGGER org_asset_reservations_immutable BEFORE DELETE OR TRUNCATE ON org_asset_reservations FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- Insert: under the organization root lock, an ACTIVE or INACTIVE asset of
-- this organization, a start not before the transaction, no removal data.
-- Update: only removing a block that is not ended, moving its end earlier
-- (never later, never before the transaction except to its own start),
-- with the command and a reason.
CREATE FUNCTION org_asset_block_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE asset_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept block changes';END IF;
 IF TG_OP='INSERT' THEN
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_blocks_asset_fkey',MESSAGE='private reference not found';END IF;
  IF asset_state NOT IN('ACTIVE','INACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept block';END IF;
  IF NEW.removed_at IS NOT NULL OR NEW.remove_operation_id IS NOT NULL OR NEW.removed_by IS NOT NULL OR NEW.removal_reason<>'' OR NEW.valid_from<transaction_timestamp() THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial block';
  END IF;
  NEW.recorded_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.asset_id,NEW.reason_kind,NEW.note,NEW.valid_from,NEW.operation_id,NEW.blocked_by,NEW.recorded_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.asset_id,OLD.reason_kind,OLD.note,OLD.valid_from,OLD.operation_id,OLD.blocked_by,OLD.recorded_at)
  OR OLD.removed_at IS NOT NULL OR NEW.valid_to IS NULL OR NEW.valid_to<OLD.valid_from OR(OLD.valid_to IS NOT NULL AND NEW.valid_to>OLD.valid_to)
  OR public.org_asset_assignment_ended(OLD.valid_from,OLD.valid_to,transaction_timestamp()) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='block already ended or immutable';
 END IF;
 IF NEW.valid_to<transaction_timestamp() AND NEW.valid_to<>OLD.valid_from OR NEW.remove_operation_id IS NULL OR NEW.removed_by IS NULL OR NEW.removal_reason='' THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='block removal source unavailable';
 END IF;
 NEW.removed_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_block_write BEFORE INSERT OR UPDATE ON org_asset_blocks FOR EACH ROW EXECUTE FUNCTION org_asset_block_guard();
CREATE TRIGGER org_asset_blocks_immutable BEFORE DELETE OR TRUNCATE ON org_asset_blocks FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- B-BR-EMP-297 (fleet.md §6.1): the transaction that takes an asset out of
-- service cancels every REQUESTED or CONFIRMED reservation of it that has not
-- ended by time, with the asset revision as the proof of the cause.
CREATE FUNCTION org_asset_reservation_left_service() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected TIMESTAMPTZ:=transaction_timestamp();
BEGIN
 UPDATE public.org_asset_reservations SET state='CANCELLED',revision=revision+1,end_cause='ASSET_LEFT_SERVICE',source_revision=NEW.revision
 WHERE org_id=NEW.org_id AND asset_id=NEW.id
  AND(state='REQUESTED' AND selected<starts_at OR state='CONFIRMED' AND selected<public.org_asset_reservation_deadline(starts_at,ends_at,no_show_tolerance));
 RETURN NULL;
END $$;
CREATE TRIGGER org_asset_reservation_left_service AFTER UPDATE OF state ON org_assets FOR EACH ROW
 WHEN(NEW.state IN('INACTIVE','DISPOSED','ARCHIVED') AND OLD.state='ACTIVE') EXECUTE FUNCTION org_asset_reservation_left_service();

-- The 215 asset-use source with the reservation and block keys recognised:
-- both name only ACTIVE (or, for a block, INACTIVE) assets, so they are never
-- use of a discardable draft and the counts are unchanged.
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
 OR(c.conrelid='public.org_asset_blocks'::regclass AND c.conname='org_asset_blocks_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset));
END $$;
