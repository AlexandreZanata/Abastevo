-- EMP-FLEET-011. Implements attached to a powered asset (B-BR-EMP-325..329,
-- EDR-210..212). Additive: one attachment table with its receipts and guards,
-- and the asset-use source with the two new foreign keys allow-listed and
-- attachments counted as asset operations (they fix the kind of both assets,
-- BR-FLEET-002). The asset-use body is the 000220 one (trips and held starts
-- kept) plus the attachment lines. Applied migrations stay unchanged; no
-- existing row is written.

-- One receipt per command (EDR-210): the attachment it created or detached.
CREATE TABLE org_asset_attachment_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL CHECK(asset_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('ATTACH','DETACH')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 attachment_id UUID NOT NULL CHECK(attachment_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_asset_attachment_commands_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TRIGGER org_asset_attachment_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_attachment_commands FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- BR-FLEET-033 / B-BR-EMP-325: an IMPLEMENT is attached to at most one powered
-- asset at any instant (half-open whole-microsecond periods); a powered
-- asset may carry several implements. An attachment detached before its
-- start keeps an empty period, which overlaps nothing.
CREATE TABLE org_asset_implement_attachments(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 implement_id UUID NOT NULL,powered_asset_id UUID NOT NULL,
 valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,
 operation_id UUID NOT NULL,
 attached_by UUID NOT NULL CHECK(attached_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 detached_at TIMESTAMPTZ,detach_operation_id UUID,
 detached_by UUID CHECK(detached_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 detach_reason TEXT NOT NULL DEFAULT '' CHECK(detach_reason='' OR org_asset_text_valid(detach_reason,280,true)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_asset_implement_attachments_implement_fkey FOREIGN KEY(org_id,implement_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_implement_attachments_powered_fkey FOREIGN KEY(org_id,powered_asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_implement_attachments_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_asset_attachment_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_implement_attachments_detach_command_fkey FOREIGN KEY(org_id,detach_operation_id) REFERENCES org_asset_attachment_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_implement_attachments_distinct_check CHECK(implement_id<>powered_asset_id),
 CONSTRAINT org_asset_implement_attachments_bounds_check CHECK(isfinite(valid_from) AND valid_from>='0001-01-01 00:00:00+00'::timestamptz AND valid_from<'10000-01-01 00:00:00+00'::timestamptz
  AND date_trunc('microseconds',valid_from)=valid_from
  AND(valid_to IS NULL OR(isfinite(valid_to) AND valid_to>=valid_from AND valid_to<'10000-01-01 00:00:00+00'::timestamptz AND date_trunc('microseconds',valid_to)=valid_to))
  AND isfinite(recorded_at) AND(detached_at IS NULL OR isfinite(detached_at))),
 CONSTRAINT org_asset_implement_attachments_period_check CHECK(detached_at IS NOT NULL OR valid_to IS NULL OR valid_to>valid_from),
 CONSTRAINT org_asset_implement_attachments_detach_check CHECK((detached_at IS NULL)=(detach_operation_id IS NULL) AND(detached_at IS NULL)=(detached_by IS NULL)
  AND(detached_at IS NOT NULL OR detach_reason='') AND(detached_at IS NULL OR valid_to IS NOT NULL)),
 CONSTRAINT org_asset_implement_attachments_no_overlap EXCLUDE USING gist(org_id WITH =,implement_id WITH =,tstzrange(valid_from,valid_to,'[)') WITH &&)
);
CREATE INDEX org_asset_implement_attachments_implement ON org_asset_implement_attachments(org_id,implement_id,valid_from,id);
CREATE INDEX org_asset_implement_attachments_powered ON org_asset_implement_attachments(org_id,powered_asset_id,valid_from,id);

DO $$
DECLARE name TEXT;
BEGIN
 FOREACH name IN ARRAY ARRAY['org_asset_attachment_commands','org_asset_implement_attachments'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- Insert: under the organization root lock, an IMPLEMENT and a VEHICLE or
-- MACHINE of this organization, both ACTIVE or INACTIVE, a start not before
-- the transaction and no detach data. Update: only detaching an attachment
-- that is not ENDED, moving its end earlier (never later, never before the
-- transaction except to its own start), with the command. Never delete.
CREATE FUNCTION org_asset_attachment_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE implement_kind TEXT;implement_state TEXT;powered_kind TEXT;powered_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept attachment changes';END IF;
 IF TG_OP='INSERT' THEN
  SELECT kind,state INTO implement_kind,implement_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.implement_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_implement_attachments_implement_fkey',MESSAGE='private reference not found';END IF;
  SELECT kind,state INTO powered_kind,powered_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.powered_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_implement_attachments_powered_fkey',MESSAGE='private reference not found';END IF;
  IF implement_kind<>'IMPLEMENT' OR powered_kind NOT IN('VEHICLE','MACHINE') OR implement_state NOT IN('ACTIVE','INACTIVE') OR powered_state NOT IN('ACTIVE','INACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assets do not accept this attachment';
  END IF;
  IF NEW.detached_at IS NOT NULL OR NEW.detach_operation_id IS NOT NULL OR NEW.detached_by IS NOT NULL OR NEW.detach_reason<>'' OR NEW.valid_from<transaction_timestamp() THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial attachment';
  END IF;
  NEW.recorded_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.implement_id,NEW.powered_asset_id,NEW.valid_from,NEW.operation_id,NEW.attached_by,NEW.recorded_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.implement_id,OLD.powered_asset_id,OLD.valid_from,OLD.operation_id,OLD.attached_by,OLD.recorded_at)
  OR OLD.detached_at IS NOT NULL OR NEW.valid_to IS NULL OR NEW.valid_to<OLD.valid_from OR(OLD.valid_to IS NOT NULL AND NEW.valid_to>OLD.valid_to)
  OR public.org_asset_assignment_ended(OLD.valid_from,OLD.valid_to,transaction_timestamp()) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='attachment already ended or immutable';
 END IF;
 IF NEW.valid_to<transaction_timestamp() AND NEW.valid_to<>OLD.valid_from OR NEW.detach_operation_id IS NULL OR NEW.detached_by IS NULL THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='attachment detach source unavailable';
 END IF;
 NEW.detached_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_attachment_write BEFORE INSERT OR UPDATE ON org_asset_implement_attachments FOR EACH ROW EXECUTE FUNCTION org_asset_attachment_guard();
CREATE TRIGGER org_asset_implement_attachments_immutable BEFORE DELETE OR TRUNCATE ON org_asset_implement_attachments FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- The 217 asset-use source with the attachment keys recognised and
-- attachments counted as asset operations: an attachment fixes the kind of
-- both assets (BR-FLEET-002/033). Neither names a DRAFT asset, so discard is
-- unaffected.
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
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_powered_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;
