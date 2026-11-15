-- EMP-FLEET-003. Asset lifecycle: acquisition, deactivation, disposal, revert
-- and archive (B-BR-EMP-202..206, EDR-143..145). Applied migrations 000001..
-- 000200 stay unchanged. Every addition is nullable or defaulted: existing
-- assets receive no invented acquisition, disposal or lifecycle history.

-- B-BR-EMP-203/204: current lifecycle data on the asset row. Instants keep the
-- meter precision profile: microsecond timestamptz plus 0..999 nanoseconds.
ALTER TABLE org_assets
 ADD COLUMN acquired_at TIMESTAMPTZ,
 ADD COLUMN acquired_submicro SMALLINT NOT NULL DEFAULT 0,
 ADD COLUMN disposal_kind TEXT,
 ADD COLUMN disposed_at TIMESTAMPTZ,
 ADD COLUMN disposed_submicro SMALLINT NOT NULL DEFAULT 0,
 ADD COLUMN disposal_reason TEXT,
 ADD CONSTRAINT org_asset_acquisition_check CHECK(acquired_submicro BETWEEN 0 AND 999 AND(acquired_at IS NOT NULL OR acquired_submicro=0)
  AND(acquired_at IS NULL OR(isfinite(acquired_at) AND acquired_at>='0001-01-01 00:00:00+00'::timestamptz AND acquired_at<'10000-01-01 00:00:00+00'::timestamptz))),
 ADD CONSTRAINT org_asset_disposal_check CHECK(disposed_submicro BETWEEN 0 AND 999 AND(
  (state IN('DISPOSED','ARCHIVED') AND disposal_kind IN('SALE','WRITE_OFF','RETURN_TO_HOLDER') AND disposed_at IS NOT NULL AND isfinite(disposed_at)
   AND disposed_at>='0001-01-01 00:00:00+00'::timestamptz AND disposed_at<'10000-01-01 00:00:00+00'::timestamptz
   AND disposal_reason IS NOT NULL AND org_asset_text_valid(disposal_reason,280,true))
  OR(state NOT IN('DISPOSED','ARCHIVED') AND disposal_kind IS NULL AND disposed_at IS NULL AND disposed_submicro=0 AND disposal_reason IS NULL)));

-- One receipt namespace for every asset command (EDR-143). The original kinds
-- keep exactly their 197 result rules; each lifecycle kind names its target.
ALTER TABLE org_asset_commands DROP CONSTRAINT org_asset_commands_kind_check,DROP CONSTRAINT org_asset_commands_check,
 ADD CONSTRAINT org_asset_commands_kind_check CHECK(kind IN('CREATE','CHANGE','DISCARD','ACTIVATE','RECORD_ACQUISITION','DEACTIVATE','REACTIVATE','DISPOSE','REVERT_DISPOSAL','ARCHIVE')),
 ADD CONSTRAINT org_asset_commands_result_check CHECK(
  (kind='DISCARD' AND discarded AND result_state='' AND result_revision>1)
  OR(kind IN('CREATE','CHANGE','ACTIVATE') AND NOT discarded AND result_state IN('DRAFT','ACTIVE') AND(kind<>'CREATE' OR(result_state='DRAFT' AND result_revision=1)))
  OR(NOT discarded AND result_revision>1 AND((kind='RECORD_ACQUISITION' AND result_state IN('DRAFT','ACTIVE','INACTIVE'))
   OR(kind='DEACTIVATE' AND result_state='INACTIVE') OR(kind='REACTIVATE' AND result_state='ACTIVE') OR(kind='DISPOSE' AND result_state='DISPOSED')
   OR(kind='REVERT_DISPOSAL' AND result_state='INACTIVE') OR(kind='ARCHIVE' AND result_state='ARCHIVED'))));

-- B-BR-EMP-204: immutable lifecycle history keyed by the revision it produced.
-- A revert never removes the original DISPOSE row.
CREATE TABLE org_asset_lifecycle_events(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,revision BIGINT NOT NULL CHECK(revision>1),
 operation_id UUID NOT NULL,actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 command TEXT NOT NULL,from_state TEXT NOT NULL,to_state TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '' CHECK(reason='' OR org_asset_text_valid(reason,280,true)),
 disposal_kind TEXT,disposed_at TIMESTAMPTZ,disposed_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(disposed_submicro BETWEEN 0 AND 999),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,asset_id,revision),UNIQUE(org_id,operation_id),
 CONSTRAINT org_asset_lifecycle_revision_fkey FOREIGN KEY(org_id,asset_id,revision) REFERENCES org_asset_revisions(org_id,asset_id,revision) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_lifecycle_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_asset_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_lifecycle_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_asset_lifecycle_row_check CHECK(
  (command='DEACTIVATE' AND from_state='ACTIVE' AND to_state='INACTIVE' AND reason<>'' AND disposal_kind IS NULL AND disposed_at IS NULL AND disposed_submicro=0)
  OR(command='REACTIVATE' AND from_state='INACTIVE' AND to_state='ACTIVE' AND reason='' AND disposal_kind IS NULL AND disposed_at IS NULL AND disposed_submicro=0)
  OR(command='DISPOSE' AND from_state IN('ACTIVE','INACTIVE') AND to_state='DISPOSED' AND reason<>'' AND disposal_kind IN('SALE','WRITE_OFF','RETURN_TO_HOLDER')
   AND disposed_at IS NOT NULL AND isfinite(disposed_at) AND disposed_at>='0001-01-01 00:00:00+00'::timestamptz AND disposed_at<'10000-01-01 00:00:00+00'::timestamptz)
  OR(command='REVERT_DISPOSAL' AND from_state='DISPOSED' AND to_state='INACTIVE' AND reason<>'' AND disposal_kind IS NULL AND disposed_at IS NULL AND disposed_submicro=0)
  OR(command='ARCHIVE' AND from_state='DISPOSED' AND to_state='ARCHIVED' AND reason='' AND disposal_kind IS NULL AND disposed_at IS NULL AND disposed_submicro=0))
);
-- A history row must describe the exact asset row and previous snapshot of
-- its own revision; disposal data must equal the current disposal it records.
CREATE FUNCTION org_asset_lifecycle_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;previous TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO a FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 SELECT snapshot->>'state' INTO previous FROM public.org_asset_revisions WHERE org_id=NEW.org_id AND asset_id=NEW.asset_id AND revision=NEW.revision-1;
 IF a.id IS NULL OR a.revision<>NEW.revision OR a.state<>NEW.to_state OR previous IS DISTINCT FROM NEW.from_state
  OR(NEW.command='DISPOSE' AND ROW(a.disposal_kind,a.disposed_at,a.disposed_submicro,a.disposal_reason) IS DISTINCT FROM ROW(NEW.disposal_kind,NEW.disposed_at,NEW.disposed_submicro,NEW.reason))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset lifecycle history source unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_asset_lifecycle_source BEFORE INSERT ON org_asset_lifecycle_events FOR EACH ROW EXECUTE FUNCTION org_asset_lifecycle_source();
CREATE TRIGGER org_asset_lifecycle_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_asset_lifecycle_events FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
ALTER TABLE org_asset_lifecycle_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_asset_lifecycle_events FORCE ROW LEVEL SECURITY;
CREATE POLICY org_asset_lifecycle_events_scope ON org_asset_lifecycle_events USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_asset_lifecycle_events FROM PUBLIC;

-- B-BR-EMP-202: every state change except FLEET-002 activation commits its
-- history row in the same transaction; no field edit can set the state. The
-- nested IF keeps the history read out of every other asset update plan.
CREATE FUNCTION org_asset_lifecycle_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.state IS DISTINCT FROM OLD.state AND OLD.state<>'DRAFT' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_asset_lifecycle_events e WHERE e.org_id=NEW.org_id AND e.asset_id=NEW.id AND e.revision=NEW.revision AND e.from_state=OLD.state AND e.to_state=NEW.state) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset lifecycle transition lacks history';
  END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_asset_lifecycle_complete AFTER UPDATE ON org_assets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_lifecycle_complete();

-- The 198 asset guard with the §6.1 lifecycle rows. DRAFT/ACTIVE behaviour,
-- identifier projection, kind lock, powered evidence and unit admission are
-- preserved; reactivation re-proves the same BR-FLEET-004 evidence.
CREATE OR REPLACE FUNCTION org_asset_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;root_state TEXT;usage JSONB;lifecycle BOOLEAN;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept asset changes';END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.state<>'DRAFT' THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
  IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  RETURN OLD;
 END IF;
 IF NEW.fleet_key IS DISTINCT FROM public.org_asset_identifier(NEW.fleet_number,false) OR NEW.plate_key IS DISTINCT FROM public.org_asset_identifier(NEW.plate,true) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset identifier projection mismatch';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'DRAFT' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assets start as draft';END IF;
  IF NEW.acquired_at IS NOT NULL OR NEW.acquired_submicro<>0 OR NEW.disposal_kind IS NOT NULL OR NEW.disposed_at IS NOT NULL OR NEW.disposed_submicro<>0 OR NEW.disposal_reason IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset lifecycle data starts empty';END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset identity or revision';END IF;
  IF OLD.state='ARCHIVED' THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='archived asset is terminal';END IF;
  lifecycle=NEW.state IS DISTINCT FROM OLD.state;
  IF lifecycle AND NOT((OLD.state='DRAFT' AND NEW.state='ACTIVE') OR(OLD.state='ACTIVE' AND NEW.state='INACTIVE') OR(OLD.state='INACTIVE' AND NEW.state='ACTIVE')
   OR(OLD.state IN('ACTIVE','INACTIVE') AND NEW.state='DISPOSED') OR(OLD.state='DISPOSED' AND NEW.state IN('INACTIVE','ARCHIVED'))) THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset transition unavailable';END IF;
  IF (lifecycle AND OLD.state<>'DRAFT' OR OLD.state IN('INACTIVE','DISPOSED'))
   AND ROW(NEW.kind,NEW.category_id,NEW.name,NEW.fleet_number,NEW.plate,NEW.serial_number,NEW.fleet_key,NEW.plate_key,NEW.usage_basis,NEW.unit_id) IS DISTINCT FROM ROW(OLD.kind,OLD.category_id,OLD.name,OLD.fleet_number,OLD.plate,OLD.serial_number,OLD.fleet_key,OLD.plate_key,OLD.usage_basis,OLD.unit_id)
  THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset lifecycle command keeps its profile';END IF;
  IF ROW(NEW.disposal_kind,NEW.disposed_at,NEW.disposed_submicro,NEW.disposal_reason) IS DISTINCT FROM ROW(OLD.disposal_kind,OLD.disposed_at,OLD.disposed_submicro,OLD.disposal_reason)
   AND NOT((lifecycle AND NEW.state='DISPOSED') OR(OLD.state='DISPOSED' AND NEW.state='INACTIVE')) THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset disposal changes only by command';END IF;
  IF lifecycle AND NEW.state='DISPOSED' AND((NEW.disposed_at,NEW.disposed_submicro)>(clock_timestamp(),0)
   OR(NEW.acquired_at IS NOT NULL AND(NEW.disposed_at,NEW.disposed_submicro)<(NEW.acquired_at,NEW.acquired_submicro))) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset disposal instant';END IF;
  IF ROW(NEW.acquired_at,NEW.acquired_submicro) IS DISTINCT FROM ROW(OLD.acquired_at,OLD.acquired_submicro) THEN
   IF lifecycle OR NEW.state NOT IN('DRAFT','ACTIVE','INACTIVE') OR NEW.acquired_at IS NULL THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset acquisition changes only by command';END IF;
   IF (NEW.acquired_at,NEW.acquired_submicro)>(clock_timestamp(),0) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset acquisition instant';END IF;
  END IF;
  IF NEW.kind<>OLD.kind THEN
   usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
   IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset kind fixed by accepted operation';END IF;
  END IF;
  IF NEW.state='ACTIVE' AND NEW.kind<>'IMPLEMENT' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_asset_tanks WHERE org_id=NEW.org_id AND asset_id=NEW.id) OR NOT EXISTS(SELECT 1 FROM public.org_meters WHERE org_id=NEW.org_id AND asset_id=NEW.id AND (installed_at,installed_submicro)<=(clock_timestamp(),0) AND removed_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset activation evidence incomplete';END IF;
  END IF;
  IF OLD.state='ACTIVE' AND NEW.unit_id IS DISTINCT FROM OLD.unit_id THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='active asset unit change unavailable';END IF;
  IF NEW.state='ACTIVE' AND OLD.state IN('DRAFT','INACTIVE') THEN
   PERFORM public.org_unit_admit(NEW.org_id,NEW.unit_id);
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 -- An out-of-service asset may keep a unit that was archived after it left
 -- service; DRAFT and ACTIVE keep the original active-unit requirement.
 IF NEW.unit_id IS NOT NULL AND NEW.state IN('DRAFT','ACTIVE') AND NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
 RETURN NEW;
END $$;

-- B-BR-EMP-206 SQL defence: the 199 guard plus one live-branch rule. An
-- accepted review (the reviewed branch) is the only way to store an
-- observation at or after the asset's current disposal instant.
CREATE OR REPLACE FUNCTION org_meter_observation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;original_received TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.segment_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation outside meter service interval';END IF;
 IF NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid meter observation';END IF;
 IF NEW.review_resolution_id IS NOT NULL THEN
  SELECT a.received_at INTO original_received FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
  JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
  JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
  WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id AND z.outcome='ACCEPTED' AND z.reading_id=NEW.id AND NEW.id=h.reading_id
  AND NEW.meter_id=h.meter_id AND NEW.segment_id=h.segment_id AND NEW.value=h.value AND(NEW.occurred_at,NEW.occurred_submicro)=(h.occurred_at,h.occurred_submicro)
  AND NEW.entry_zone=h.entry_zone AND NEW.device_offset_seconds=h.device_offset_seconds AND NEW.operation_id=o.operation_id AND NEW.request_hash=h.request_hash
  AND NEW.author_membership_id=o.membership_id AND NEW.capture_kind='ENROLLED_DEVICE' AND NEW.enrollment_id=o.enrollment_id AND NEW.web_session_id IS NULL AND NEW.origin=h.origin AND NEW.method=h.method AND NEW.state='VALID';
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewed observation source unavailable';END IF;
  NEW.received_at=original_received;RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM public.org_meter_held_readings WHERE org_id=NEW.org_id AND(reading_id=NEW.id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=m.asset_id AND a.disposed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(a.disposed_at,a.disposed_submicro)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation after asset disposal requires review';END IF;
 IF NEW.capture_kind='ENROLLED_DEVICE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.enrollment_id AND membership_id=NEW.author_membership_id AND state='ENROLLED') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.company_web_sessions c JOIN public.org_memberships p ON p.account_id=c.account_id WHERE p.org_id=NEW.org_id AND p.id=NEW.author_membership_id AND c.id=NEW.web_session_id) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- B-BR-EMP-206: the 199 held-source guard plus one typed proof. An
-- ASSET_UNAVAILABLE held fact needs an actual disposal at or before it.
CREATE OR REPLACE FUNCTION org_meter_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;i public.org_sync_review_items%ROWTYPE;m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;actual_unit UUID;disposal_at TIMESTAMPTZ;disposal_sub SMALLINT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR i.operation_type<>'meter.reading.record' OR i.operation_schema<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter intent unavailable';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR o.entity_type<>'meter_reading' OR o.entity_id<>NEW.reading_id OR NOT EXISTS(SELECT 1 FROM public.org_device_enrollments e JOIN public.org_memberships p ON(p.org_id,p.id)=(e.org_id,e.membership_id) WHERE e.org_id=o.org_id AND e.id=o.enrollment_id AND e.membership_id=o.membership_id AND e.state='ENROLLED' AND p.state='ACTIVE' AND p.account_id=o.owner_account_id AND p.employee_id=i.author_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter source unavailable';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 -- The held fact reserves its reading identity; it cannot claim an applied one.
 IF EXISTS(SELECT 1 FROM public.org_meter_readings WHERE org_id=NEW.org_id AND(id=NEW.reading_id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 SELECT unit_id,disposed_at,disposed_submicro INTO actual_unit,disposal_at,disposal_sub FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND OR NEW.unit_id IS DISTINCT FROM actual_unit THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter context unavailable';END IF;
 IF i.code='ASSET_UNAVAILABLE' AND(disposal_at IS NULL OR(NEW.occurred_at,NEW.occurred_submicro)<(disposal_at,disposal_sub)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held asset unavailability unproven';END IF;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id AND asset_id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND id=NEW.segment_id AND meter_id=NEW.meter_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 OR NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held meter observation';END IF;
 RETURN NEW;
END $$;
